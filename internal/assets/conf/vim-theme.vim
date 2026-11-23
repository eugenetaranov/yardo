" yardo: Vim's colours, as yardo's Settings › Look › Editor theme picked
" them (~/.config/yardo/editor-theme names one for Vim and Neovim). A
" colorscheme in your own ~/.vimrc wins; without a pick, Vim's own.
"
" Written by yardo, which rewrites it on a re-run.

if exists('g:loaded_yardo_theme') || exists('g:colors_name')
  finish
endif
let g:loaded_yardo_theme = 1

let s:file = expand('~/.config/yardo/editor-theme')
if !filereadable(s:file)
  finish
endif
for s:line in readfile(s:file)
  let s:name = matchstr(s:line, '^vim\s\+\zs\S\+')
  if !empty(s:name)
    if has('termguicolors') && $COLORTERM =~# 'truecolor\|24bit'
      set termguicolors
    endif
    syntax enable
    set background=dark
    execute 'silent! colorscheme' s:name
  endif
endfor
