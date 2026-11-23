" yardo: what you yank in Vim lands in your computer's clipboard, over
" SSH too (inside tmux and zellij as well): Vim sends it to your terminal
" through OSC 52, as yardo's Neovim does. Paste it with cmd+v / ctrl+v;
" a delete (dd, x) doesn't touch it.
"
" With no ~/.vimrc of your own the mouse is the terminal's too: select
" and copy as you would anywhere else (set mouse=a in ~/.vimrc for Vim's).
"
" Written by yardo, which rewrites it on a re-run; your own settings go
" in ~/.vimrc. let g:loaded_yardo_clipboard = 1 there turns this off.

if exists('g:loaded_yardo_clipboard') || !exists('##TextYankPost') || !exists('*echoraw')
  finish
endif
let g:loaded_yardo_clipboard = 1

" On a desktop, with a clipboard Vim can reach, "+y already copies.
if empty($SSH_TTY) && empty($SSH_CONNECTION) && has('clipboard_working')
  finish
endif

if !filereadable(expand('~/.vimrc')) && !filereadable(expand('~/.vim/vimrc'))
  set mouse=
endif

function! s:Copy(event) abort
  if a:event.operator !=# 'y' || index(['', '+', '*'], a:event.regname) < 0
    return
  endif
  let text = join(a:event.regcontents, "\n")
  if a:event.regtype ==# 'V'
    let text .= "\n"
  endif
  let b64 = substitute(system('base64', text), '\n', '', 'g')
  if v:shell_error == 0
    call echoraw("\e]52;c;" . b64 . "\x07")
  endif
endfunction

augroup yardo_clipboard
  autocmd!
  autocmd TextYankPost * call s:Copy(v:event)
augroup END
