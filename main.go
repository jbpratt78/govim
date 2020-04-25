// Command govim is a Vim8 channel-based plugin, written in Go, to support the writing of Go code in Vim8
package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/govim/govim"
	"github.com/govim/govim/testsetup"
	"github.com/jbpratt78/vimcollab/config"
	"github.com/jbpratt78/vimcollab/internal/plugin"
	"github.com/jbpratt78/vimcollab/internal/types"
	"gopkg.in/tomb.v2"
)

const (
	PluginPrefix = "GOVIM"
)

var (
	// exposeTestAPI is a rather hacky but clean way of only exposing certain
	// functions, commands and autocommands to Vim when run from a test
	exposeTestAPI = os.Getenv(testsetup.EnvLoadTestAPI) == "true"
)

func mainerr() error {
	if err := flagSet.Parse(os.Args[1:]); err != nil {
		return flagErr(err.Error())
	}

	if sock := os.Getenv(testsetup.EnvTestSocket); sock != "" {
		ln, err := net.Listen("tcp", sock)
		if err != nil {
			return fmt.Errorf("failed to listen on %v: %v", sock, err)
		}
		for {
			conn, err := ln.Accept()
			if err != nil {
				return fmt.Errorf("failed to accept connection on %v: %v", sock, err)
			}

			go func() {
				if err := launch(conn, conn); err != nil {
					fmt.Fprintln(os.Stderr, err)
				}
			}()
		}
	} else {
		return launch(os.Stdin, os.Stdout)
	}
}

func launch(in io.ReadCloser, out io.WriteCloser) error {
	defer out.Close()

	d := newplugin()

	tf, err := d.createLogFile("govim")
	if err != nil {
		return err
	}
	defer tf.Close()

	var log io.Writer = tf
	if *fTail {
		log = io.MultiWriter(tf, os.Stdout)
	}

	if os.Getenv(testsetup.EnvTestSocket) != "" {
		fmt.Fprintf(os.Stderr, "New connection will log to %v\n", tf.Name())
	}

	g, err := govim.NewGovim(d, in, out, log, &d.tomb)
	if err != nil {
		return fmt.Errorf("failed to create govim instance: %v", err)
	}

	d.tomb.Go(g.Run)
	return d.tomb.Wait()
}

func (g *govimplugin) createLogFile(prefix string) (*os.File, error) {
	var tf *os.File
	var err error
	logfiletmpl := os.Getenv("VIMCOLLAB_LOGFILE_TMPL")
	if logfiletmpl == "" {
		logfiletmpl = "%v_%v_%v"
	}
	logfiletmpl += ".log"
	logfiletmpl = strings.Replace(logfiletmpl, "%v", prefix, 1)
	logfiletmpl = strings.Replace(logfiletmpl, "%v", time.Now().Format("20060102_1504_05"), 1)
	if strings.Contains(logfiletmpl, "%v") {
		logfiletmpl = strings.Replace(logfiletmpl, "%v", "*", 1)
		tf, err = ioutil.TempFile(g.tmpDir, logfiletmpl)
	} else {
		// append to existing file
		tf, err = os.OpenFile(filepath.Join(g.tmpDir, logfiletmpl), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	}
	if err != nil {
		err = fmt.Errorf("failed to create log file: %v", err)
	}
	return tf, err
}

type govimplugin struct {
	plugin.Driver
	vimstate *vimstate

	// errCh is the channel passed from govim on Init
	errCh chan error

	// tmpDir is the temp directory within which log files will be created
	tmpDir string

	isGui bool

	tomb tomb.Tomb

	bufferUpdates chan *bufferUpdate

	// inShutdown is closed when govim is told to Shutdown
	inShutdown chan struct{}
}

func newplugin() *govimplugin {
	d := plugin.NewDriver(PluginPrefix)
	res := &govimplugin{
		Driver:     d,
		inShutdown: make(chan struct{}),
		vimstate: &vimstate{
			Driver:  d,
			buffers: make(map[int]*types.Buffer),
		},
	}
	res.vimstate.govimplugin = res
	return res
}

func (g *govimplugin) Init(gg govim.Govim, errCh chan error) error {
	g.errCh = errCh
	g.Driver.Govim = gg
	g.vimstate.Driver.Govim = gg.Scheduled()
	g.vimstate.workingDirectory = g.ParseString(g.ChannelCall("getcwd", -1))
	g.DefineAutoCommand("", govim.Events{govim.EventBufUnload}, govim.Patterns{"*"}, false, g.vimstate.bufUnload, "eval(expand('<abuf>'))")
	g.DefineAutoCommand("", govim.Events{govim.EventBufRead, govim.EventBufNewFile}, govim.Patterns{"*"}, false, g.vimstate.bufReadPost, exprAutocmdCurrBufInfo)
	g.DefineAutoCommand("", govim.Events{govim.EventBufWritePost}, govim.Patterns{"*"}, false, g.vimstate.bufWritePost, "eval(expand('<abuf>'))")
	g.DefineAutoCommand("", govim.Events{govim.EventBufDelete}, govim.Patterns{"*"}, false, g.vimstate.deleteCurrentBuffer, "eval(expand('<abuf>'))")
	g.DefineFunction(string(config.FunctionBufChanged), []string{"bufnr", "start", "end", "added", "changes"}, g.vimstate.bufChanged)
	g.DefineFunction(string(config.FunctionSetUserBusy), []string{"isBusy", "cursorPos"}, g.vimstate.setUserBusy)

	g.DefineCommand(string(config.CommandStartSession), g.vimstate.startSession, govim.NArgsZeroOrOne)
	g.DefineCommand(string(config.CommandEndSession), g.vimstate.endSesssion, govim.NArgsZeroOrOne)

	g.DefineCommand(string(config.CommandJoinSession), g.vimstate.joinSession, govim.NArgsZeroOrOne)
	g.DefineCommand(string(config.CommandLeaveSession), g.vimstate.leaveSession, govim.NArgsZeroOrOne)

	g.startProcessBufferUpdates()

	g.InitTestAPI()

	g.isGui = g.ParseInt(g.ChannelExpr(`has("gui_running")`)) == 1

	return nil
}

func (g *govimplugin) Shutdown() error {
	close(g.inShutdown)
	close(g.bufferUpdates)

	// Probably need to also close any connections here
	return nil
}
