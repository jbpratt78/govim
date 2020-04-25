call plug#begin()
Plug '/home/jbpratt/projects/vimcollab'
call plug#end()

set nocompatible
set nobackup
set nowritebackup
set noswapfile

set mouse=a

set ttymouse=sgr

set updatetime=500

set balloondelay=250

set signcolumn=yes

set autoindent
set smartindent
filetype indent on

set backspace=2

if has("patch-8.1.1904")
  set completeopt+=popup
  set completepopup=align:menu,border:off,highlight:Pmenu
endif
