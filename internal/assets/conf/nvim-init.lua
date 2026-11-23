-- yardo's Neovim: comfortable from the first start, without knowing the
-- plugins. <space>? shows this list (/ searches it, q closes it); press
-- <space> and wait, and which-key lists the keys that follow.
--
-- Find                                  <space> and one key
--   <space>f  a file                    <space>/  text in files
--   <space>b  open files                <space>r  recent files
--   <space>s  symbols in this file      <space>S  symbols in the project
--   <space>e  browse files (edit it like text; - goes up a directory);
--             nvim with no file opens it too
--   <space>d  problems                  <space>h  Neovim's help
--   <space>R  replace in files, every change shown first (<space>r in it
--             replaces, g? its keys)
--   File names in these lists are coloured by type (.tf green, .tfvars
--   yellow, YAML blue, JSON orange, Go blue-cyan, Python teal, JS/TS
--   pink); in <space>/ the line too: purple declares, orange sets a value,
--   cyan uses a Terraform var., local., module. or data.
--
-- Code                                  with a language server
--   gd   the definition                 grr  where it's used
--   K    the docs                       grn  rename
--   gra  fixes for this line            gcc  comment the line
--   ]d [d  next / previous problem
--   <space>[  back to where you jumped from (gd, a file you opened, a
--             search...), across files; <space>] forward again
--   Go and Terraform are formatted when you save.
--
-- Git                                   <space>g and one key
--   <space>gb  who changed this line    <space>gB  ...each line (q closes)
--   <space>gh  this file's history      <space>gd  diff with the last commit
--   <space>gc  changed files            <space>gs  stage this change
--   <space>gp  preview this change      <space>gu  undo this change
--   ]h [h      next / previous change
--   <space>gy  copy the link to the line on GitHub, GitLab, Bitbucket...
--   <space>go  open it in the browser
--
-- Edit
--   <space>w  save                      <space>y  copy to the clipboard
--   y copies to your computer's clipboard too (over SSH as well): paste
--   it with cmd+v / ctrl+v. The mouse is the terminal's: select and copy
--   as you would anywhere else.
--   Completion: up/down to pick, <CR> to accept (only once you've picked).
--   ( [ { " ' ` close themselves (no thanks: vim.g.minipairs_disable =
--   true in local.lua).
--   <Esc><Esc>  leave a :terminal       <space>m  Mason, more servers
--
-- Language servers are ready for JSON, YAML, shell and the languages
-- yardo installed (Go, Python, TypeScript, Java, Rust, Terraform,
-- Dockerfiles, Helm): errors are underlined as you type.
--
-- yardo rewrites this file on a re-run. Your own settings go in
-- ~/.config/nvim/local.lua: yardo never writes it, and it's read last.

-- --- Options -----------------------------------------------------------------

vim.g.mapleader = " "
vim.g.maplocalleader = " "

local o = vim.opt
o.number = true
o.mouse = "" -- the terminal selects and copies, not Neovim (mouse = "a" in local.lua for Neovim's)
o.showmode = false -- the status line shows it
o.breakindent = true
o.linebreak = true
o.undofile = true
o.ignorecase = true
o.smartcase = true
o.signcolumn = "yes"
o.updatetime = 250
o.timeoutlen = 400
o.splitright = true
o.splitbelow = true
o.list = true
o.listchars = { tab = "  ", trail = "·", nbsp = "␣" } -- only stray spaces show
o.inccommand = "split"
o.cursorline = true
o.scrolloff = 8
o.confirm = true
o.expandtab = true
o.shiftwidth = 4
o.tabstop = 4
o.softtabstop = -1
o.shiftround = true

-- Config and markup files indent by 2; vim-sleuth follows what a file
-- already uses either way.
vim.api.nvim_create_autocmd("FileType", {
  pattern = { "yaml", "json", "jsonc", "toml", "terraform", "hcl", "sh", "bash", "zsh", "lua", "html", "css", "javascript", "typescript", "markdown" },
  callback = function() vim.opt_local.shiftwidth = 2 end,
})

-- --- Keys --------------------------------------------------------------------

local map = vim.keymap.set
map("n", "<Esc>", "<cmd>nohlsearch<CR>", { desc = "Clear the search highlight" })
map("n", "<C-h>", "<C-w>h", { desc = "Window left" })
map("n", "<C-j>", "<C-w>j", { desc = "Window below" })
map("n", "<C-k>", "<C-w>k", { desc = "Window above" })
map("n", "<C-l>", "<C-w>l", { desc = "Window right" })
-- Back and forward through where you've jumped, across files: <C-o> and
-- <C-i> are Neovim's, but zellij takes <C-o> for itself.
map("n", "<leader>[", "<C-o>", { desc = "Back to where you jumped from" })
map("n", "<leader>]", "<C-i>", { desc = "Forward again" })
map("n", "<leader>w", "<cmd>write<CR>", { desc = "Save" })
map({ "n", "x" }, "<leader>y", '"+y', { desc = "Copy to the clipboard" })
map("n", "<leader>Y", '"+y$', { desc = "Copy to the end of the line to the clipboard" })
map("t", "<Esc><Esc>", "<C-\\><C-n>", { desc = "Leave terminal mode" })
map("n", "<leader>m", "<cmd>Mason<CR>", { desc = "Install language servers (Mason)" })

-- <space>?: the keys listed at the top of this file, in a window of their
-- own: / searches them, q closes it.
local function keys_help()
  local lines = {}
  for line in io.lines((debug.getinfo(1, "S").source:gsub("^@", ""))) do
    if not line:match("^%-%-") then break end
    table.insert(lines, (line:gsub("^%-%- ?", "")))
  end
  local width = 0
  for _, l in ipairs(lines) do width = math.max(width, vim.fn.strdisplaywidth(l)) end
  width = math.min(width + 2, vim.o.columns - 4)
  local height = math.min(#lines, vim.o.lines - 4)
  local buf = vim.api.nvim_create_buf(false, true)
  vim.api.nvim_buf_set_lines(buf, 0, -1, false, lines)
  vim.bo[buf].modifiable = false
  vim.api.nvim_open_win(buf, true, {
    relative = "editor", style = "minimal", border = "rounded", title = " Keys ",
    width = width, height = height,
    row = math.floor((vim.o.lines - height) / 2) - 1, col = math.floor((vim.o.columns - width) / 2),
  })
  vim.fn.matchadd("Title", [[^\S.*]])
  vim.fn.matchadd("Special", [[<space>\S*\|\<g[a-zA-Z]\{1,2}\>\|\<K\>\|[][][dh]\>]])
  for _, k in ipairs({ "q", "<Esc>" }) do
    map("n", k, "<cmd>close<CR>", { buffer = buf })
  end
end
map("n", "<leader>?", keys_help, { desc = "The keys" })

vim.diagnostic.config({ severity_sort = true, virtual_text = true, float = { border = "rounded" } })

-- With no screen of its own (over SSH) the clipboard is your computer's,
-- through OSC 52: Neovim sends what you copy to your terminal, inside tmux
-- too. Reading it back is something most terminals refuse, so "+p pastes
-- your last yank; paste from your computer with the terminal's paste.
if vim.env.SSH_TTY or vim.env.SSH_CONNECTION or not (vim.env.DISPLAY or vim.env.WAYLAND_DISPLAY) then
  local osc52 = require("vim.ui.clipboard.osc52")
  local function last() return { vim.fn.split(vim.fn.getreg('"'), "\n"), vim.fn.getregtype('"') } end
  vim.g.clipboard = {
    name = "OSC 52",
    copy = { ["+"] = osc52.copy("+"), ["*"] = osc52.copy("*") },
    paste = { ["+"] = last, ["*"] = last },
  }
end

-- Flash what was yanked, and copy it to your clipboard too (a yank, not
-- a delete: dd doesn't touch it).
vim.api.nvim_create_autocmd("TextYankPost", {
  callback = function()
    vim.hl.on_yank()
    local e = vim.v.event
    if e.operator == "y" and e.regname == "" and vim.fn.has("clipboard") == 1 then
      pcall(vim.fn.setreg, "+", e.regcontents, e.regtype)
    end
  end,
})

-- File names in the lists (files, recent, open, changed, find in files,
-- where it's used) are coloured by type: .tf green, .tfvars yellow, YAML
-- blue, JSON orange, Go blue-cyan, Python teal, JavaScript and TypeScript
-- pink. Each colour is a highlight group, so
-- vim.api.nvim_set_hl(0, "FileTf", { fg = "#..." }) in local.lua
-- changes one.
local colours = {
  FileTf = "String", FileTfvars = "DiagnosticWarn", FileYaml = "Function", FileJson = "Constant",
  FileGo = "Type", FilePython = "DiagnosticHint", FileJs = "@variable.builtin",
  FindDeclares = "Statement", FindUses = "DiagnosticInfo", FindSets = "Number",
}
local file_colour = { tf = "FileTf", tfvars = "FileTfvars", yaml = "FileYaml", yml = "FileYaml",
  json = "FileJson", go = "FileGo", py = "FilePython", js = "FileJs", mjs = "FileJs", cjs = "FileJs",
  jsx = "FileJs", ts = "FileJs", tsx = "FileJs" }

local function set_colours()
  for group, link in pairs(colours) do
    vim.api.nvim_set_hl(0, group, { link = link, default = true })
  end
end

-- Wraps a telescope entry maker so the file name in each line takes its
-- type's colour. `make` is called on first use: the picker fills in the
-- options it needs (column widths...) before it makes the first entry.
local function coloured(make)
  set_colours()
  local inner
  return function(item)
    inner = inner or make()
    local e = inner(item)
    local file = e and (e.filename or e.path or (type(e.value) == "string" and e.value))
    local hl_name = file and file_colour[file:match("%.(%w+)$") or ""]
    if not hl_name then return e end
    local show = e.display
    e.display = function(self, ...)
      local s, hl = show, nil
      if type(show) == "function" then s, hl = show(self, ...) end
      hl = hl or {}
      for _, name in ipairs({ vim.fn.fnamemodify(file, ":."), vim.fn.fnamemodify(file, ":~"), file,
        vim.fn.fnamemodify(file, ":t") }) do
        local from, to = s:find(name, 1, true)
        if from then
          table.insert(hl, { { from - 1, to }, hl_name })
          break
        end
      end
      return s, hl
    end
    return e
  end
end

-- A telescope list with its file names coloured: maker is the name of
-- telescope's own entry maker for that list, wrapped while the list is
-- built so it gets the options telescope prepared (its own copy of them).
local function list(picker, maker)
  return function()
    local make_entry = require("telescope.make_entry")
    local own = make_entry[maker]
    make_entry[maker] = function(opts) return coloured(function() return own(opts) end) end
    local ok, err = pcall(require("telescope.builtin")[picker])
    make_entry[maker] = own
    if not ok then error(err, 0) end
  end
end

-- grr: where it's used, from a use as well as from where it's declared,
-- in a picker with a preview. Asked from the declaration: terraform-ls
-- lists the uses only from there (var.x finds nothing, variable "x" does).
local function usages()
  local buf = vim.api.nvim_get_current_buf()
  local client = vim.lsp.get_clients({ bufnr = buf, method = "textDocument/references" })[1]
  if not client then
    return vim.notify("No language server here", vim.log.levels.WARN)
  end
  local params = vim.lsp.util.make_position_params(0, client.offset_encoding)
  client:request("textDocument/definition", params, function(_, def)
    def = def and (def[1] or def)
    if def and (def.targetUri or def.uri) then
      params = {
        textDocument = { uri = def.targetUri or def.uri },
        position = (def.targetSelectionRange or def.range).start,
      }
    end
    params.context = { includeDeclaration = false }
    client:request("textDocument/references", params, function(_, refs)
      if not refs or #refs == 0 then
        return vim.notify("No uses found (a language server reads the project first: try again in a moment)")
      end
      local finders, pickers = require("telescope.finders"), require("telescope.pickers")
      local make_entry, conf = require("telescope.make_entry"), require("telescope.config").values
      local opts = {
        layout_strategy = "vertical",
        layout_config = { width = 0.9, height = 0.8, preview_height = 0.4, prompt_position = "top", mirror = true },
        sorting_strategy = "ascending",
      }
      pickers.new(opts, {
        prompt_title = "Where it's used",
        finder = finders.new_table({
          results = vim.lsp.util.locations_to_items(refs, client.offset_encoding),
          entry_maker = coloured(function() return make_entry.gen_from_quickfix(opts) end),
        }),
        previewer = conf.qflist_previewer(opts),
        sorter = conf.generic_sorter(opts),
      }):find()
    end, buf)
  end, buf)
end

-- With a language server: gd goes to the definition, grr lists where
-- it's used in a picker with a preview (Enter goes there, Esc closes it;
-- K, grn, gra are Neovim's own), and Go is formatted, imports included,
-- on save.
vim.api.nvim_create_autocmd("LspAttach", {
  callback = function(ev)
    map("n", "gd", vim.lsp.buf.definition, { buffer = ev.buf, desc = "Go to the definition" })
    map("n", "grr", usages, { buffer = ev.buf, desc = "Where it's used" })
  end,
})
vim.api.nvim_create_autocmd("BufWritePre", {
  pattern = "*.go",
  callback = function(ev)
    if #vim.lsp.get_clients({ bufnr = ev.buf, name = "gopls" }) == 0 then
      return
    end
    vim.lsp.buf.code_action({ context = { only = { "source.organizeImports" }, diagnostics = {} }, apply = true })
    vim.lsp.buf.format({ bufnr = ev.buf, async = false })
  end,
})
-- Terraform too, as `terraform fmt` does it (given longer: terraform-ls
-- answers after what it's still reading).
vim.api.nvim_create_autocmd("BufWritePre", {
  pattern = { "*.tf", "*.tfvars" },
  callback = function(ev)
    if #vim.lsp.get_clients({ bufnr = ev.buf, name = "terraformls" }) > 0 then
      vim.lsp.buf.format({ bufnr = ev.buf, name = "terraformls", async = false, timeout_ms = 5000 })
    end
  end,
})

-- <space>s: what this file declares. In Terraform its blocks, as
-- IntelliJ's Structure lists them (resource "aws_s3_bucket" "logs",
-- variable "region"...), not every attribute inside them.
local function symbols()
  local buf = vim.api.nvim_get_current_buf()
  local client = vim.lsp.get_clients({ bufnr = buf, name = "terraformls" })[1]
  if not client then
    return require("telescope.builtin").lsp_document_symbols()
  end
  client:request("textDocument/documentSymbol", { textDocument = { uri = vim.uri_from_bufnr(buf) } }, function(_, result)
    local file, items = vim.api.nvim_buf_get_name(buf), {}
    for _, s in ipairs(result or {}) do
      local at = (s.selectionRange or s.range or s.location.range).start
      table.insert(items, { filename = file, lnum = at.line + 1, col = at.character + 1, text = s.name })
    end
    local finders, pickers = require("telescope.finders"), require("telescope.pickers")
    local make_entry, conf = require("telescope.make_entry"), require("telescope.config").values
    local opts = {
      path_display = "hidden",
      layout_strategy = "vertical",
      layout_config = { width = 0.9, height = 0.8, preview_height = 0.4, prompt_position = "top", mirror = true },
      sorting_strategy = "ascending",
    }
    pickers.new(opts, {
      prompt_title = "In this file",
      finder = finders.new_table({ results = items, entry_maker = make_entry.gen_from_quickfix(opts) }),
      previewer = conf.qflist_previewer(opts),
      sorter = conf.generic_sorter(opts),
    }):find()
  end, buf)
end

-- <space>gh: this file's commits, the change each one made in the
-- preview; Enter opens that change beside the file (q closes it).
-- Telescope's own Enter would check the file out as it was then.
local function file_history()
  local file = vim.api.nvim_buf_get_name(0)
  require("telescope.builtin").git_bcommits({
    attach_mappings = function()
      local actions, state = require("telescope.actions"), require("telescope.actions.state")
      actions.select_default:replace(function(bufnr)
        local sha = state.get_selected_entry().value
        actions.close(bufnr)
        local out = vim.fn.systemlist({ "git", "-C", vim.fs.dirname(file), "show", sha, "--", file })
        vim.cmd("vnew")
        vim.api.nvim_buf_set_lines(0, 0, -1, false, out)
        vim.bo.buftype, vim.bo.bufhidden, vim.bo.filetype = "nofile", "wipe", "git"
        vim.bo.modifiable = false
        map("n", "q", "<cmd>close<CR>", { buffer = true })
      end)
      return true
    end,
  })
end

-- --- Language servers --------------------------------------------------------

-- What yardo sets up: { server, its Mason package, the command that says
-- its language is installed (none: always) }.
local servers = {
  { "jsonls", "json-lsp" },
  { "yamlls", "yaml-language-server" },
  { "bashls", "bash-language-server" },
  { "pyright", "pyright", "uv" },
  { "ts_ls", "typescript-language-server", "tsc" },
  { "jdtls", "jdtls", "java" },
  { "terraformls", "terraform-ls", "terraform" },
  { "dockerls", "dockerfile-language-server", "docker" },
  { "helm_ls", "helm-ls", "helm" },
}
-- Servers that come with their language's yardo item: used as they are.
local own = { gopls = "gopls", ruff = "ruff", rust_analyzer = "rust-analyzer" }

-- The Mason packages wanted here and not installed yet.
local function missing()
  local out = {}
  for _, s in ipairs(servers) do
    local dir = vim.fn.stdpath("data") .. "/mason/packages/" .. s[2]
    if (not s[3] or vim.fn.executable(s[3]) == 1) and not vim.uv.fs_stat(dir) then
      table.insert(out, s[2])
    end
  end
  return out
end

-- yardo runs `nvim --headless +YardoServers +qa` (installs what's missing,
-- blocking) and `+"YardoServers check"` (exits 1 when something is).
vim.api.nvim_create_user_command("YardoServers", function(c)
  local m = missing()
  if c.args == "check" then
    vim.cmd(#m == 0 and "qall!" or "cquit! 1")
  elseif #m > 0 then
    vim.cmd("MasonInstall " .. table.concat(m, " "))
  end
end, { nargs = "?" })

-- --- Find in files -------------------------------------------------------------

-- <space>/: search the text in this directory and below, as IntelliJ's
-- Find in Files does. What you type is matched as it is (/, ., ( and all;
-- <C-r> switches to a regex and back), hidden files too, but not what
-- .gitignore leaves out or the directories below. Results come grouped,
-- as IntelliJ lists usages: where it's declared, where it's used, where a
-- value is set, other lines, then lines that are only a comment (dimmed);
-- by file and line within each. Enter goes there, Esc closes it.
local skip_dirs = { ".git", ".terraform", ".terragrunt-cache", "node_modules", ".venv", "venv",
  ".pip", "__pycache__", ".tox", ".mypy_cache", ".pytest_cache", ".ruff_cache", ".cache", ".idea" }

local function comment_only(text)
  return text:match("^%s*//") or text:match("^%s*/%*") or text:match("^%s*%*[%s/]") or text:match("^%s*%*$")
    or text:match("^%s*#[%s#!]") or text:match("^%s*#$") or text:match("^%s*%-%-")
    or text:match("^%s*<!%-%-") or text:match("^%s*;")
end

-- The matched line is coloured by what it does (the file name by type, as
-- in every list): purple declares something (variable "x", resource,
-- module in Terraform; def, class in Python; func, type, var, const in Go;
-- function, class, const, let in JavaScript), orange sets a value (an
-- assignment, a .tfvars line, a YAML or JSON key), cyan uses a Terraform
-- name (var.x, local.x, module.x, data.x). Other lines stay plain.
local line_rules = {
  tf = {
    declares = { '^%s*variable%s+"', '^%s*resource%s+"', '^%s*module%s+"', '^%s*output%s+"', '^%s*data%s+"' },
    uses = { "%f[%w_]var%.[%a_]", "%f[%w_]local%.[%a_]", "%f[%w_]module%.[%a_]", "%f[%w_]data%.[%a_]" },
  },
  tfvars = { sets = { "^%s*[%w_-]+%s*=" } },
  py = {
    declares = { "^%s*def%s", "^%s*async%s+def%s", "^%s*class%s" },
    sets = { "^%s*[%w_.,%s]+=[^=]", "^%s*[%w_.]+%s*:%s*[^=]+=[^=]" },
  },
  go = {
    declares = { "^%s*func%s", "^%s*type%s", "^%s*var%s", "^%s*const%s" },
    sets = { ":=", "^%s*[%w_.,%s]+=[^=]" },
  },
  js = {
    declares = { "^%s*export%s+default%s", "^%s*export%s+[%a%s]*function", "^%s*export%s+[%a%s]*class%s",
      "^%s*export%s+const%s", "^%s*export%s+let%s", "^%s*async%s+function", "^%s*function",
      "^%s*class%s", "^%s*const%s", "^%s*let%s", "^%s*var%s", "^%s*interface%s", "^%s*type%s" },
    sets = { "^%s*[%w_.$]+%s*=[^=>]" },
  },
  yaml = { sets = { "^%s*%-?%s*[%w_.\"'/-]+%s*:" } },
  json = { sets = { '^%s*"[^"]+"%s*:' } },
}
line_rules.yml = line_rules.yaml
for _, js in ipairs({ "mjs", "cjs", "jsx", "ts", "tsx" }) do line_rules[js] = line_rules.js end

local function line_colour(ext, text)
  local rules = line_rules[ext]
  if not rules then return nil end
  for _, kind in ipairs({ "declares", "uses", "sets" }) do
    for _, pattern in ipairs(rules[kind] or {}) do
      if text:match(pattern) then
        return ({ declares = "FindDeclares", uses = "FindUses", sets = "FindSets" })[kind]
      end
    end
  end
end

local function find_in_files()
  local finders, pickers = require("telescope.finders"), require("telescope.pickers")
  local make_entry, conf = require("telescope.make_entry"), require("telescope.config").values
  local regex = false
  -- --sort path: files in the same order every time (ripgrep otherwise
  -- lists them as its threads finish).
  local args = { "rg", "--color=never", "--no-heading", "--with-filename", "--line-number",
    "--column", "--smart-case", "--hidden", "--sort", "path" }
  for _, d in ipairs(skip_dirs) do
    vim.list_extend(args, { "--glob", "!**/" .. d .. "/**" })
  end
  set_colours()

  local base = make_entry.gen_from_vimgrep({})
  local function entry(line)
    local e = base(line)
    if not e then return e end
    local show = e.display
    if comment_only(e.text or "") then
      e.rank = 5
      e.display = function(self, ...)
        local s, hl = show(self, ...)
        return s, vim.list_extend({ { { 0, #s }, "Comment" } }, hl or {})
      end
      return e
    end
    local ext = (e.filename or ""):match("%.(%w+)$")
    local name_hl, text_hl = file_colour[ext], line_colour(ext, e.text or "")
    e.rank = ({ FindDeclares = 1, FindUses = 2, FindSets = 3 })[text_hl] or 4
    if name_hl or text_hl then
      e.display = function(self, ...)
        local s, hl = show(self, ...)
        hl = hl or {}
        local prefix = s:match("^(.-:%d+:%d+:)")
        if prefix then
          local name = prefix:match("^(.*):%d+:%d+:$")
          if name_hl then table.insert(hl, { { 0, #name }, name_hl }) end
          if text_hl then table.insert(hl, { { #prefix, #s }, text_hl }) end
        end
        return s, hl
      end
    end
    return e
  end

  local finder = finders.new_job(function(prompt)
    if not prompt or prompt == "" then return nil end
    return vim.iter({ args, regex and {} or { "--fixed-strings" }, "--", prompt }):flatten():totable()
  end, entry, nil, vim.uv.cwd())

  local fzy = require("telescope.algos.fzy")
  pickers.new({
    layout_strategy = "vertical",
    layout_config = { width = 0.9, height = 0.9, preview_height = 0.45, prompt_position = "top" },
    sorting_strategy = "ascending",
  }, {
    prompt_title = "Find in files (as typed · <C-r> regex)",
    finder = finder,
    previewer = conf.grep_previewer({}),
    -- By group (see above); within one, in the order ripgrep sorted them.
    sorter = require("telescope.sorters").Sorter:new({
      scoring_function = function(_, _, _, e) return e.rank or 4 end,
      -- What you typed, where it is in the line (not scattered over the
      -- file name, as a fuzzy match would be); a regex keeps the fuzzy one.
      highlighter = function(_, prompt, display)
        if regex or prompt == "" then return fzy.positions(prompt, display) end
        local caseless = prompt == prompt:lower()
        local hay, needle = caseless and display:lower() or display, caseless and prompt:lower() or prompt
        local start, out = #(display:match("^.-:%d+:%d+:") or "") + 1, {}
        while true do
          local from, to = hay:find(needle, start, true)
          if not from then break end
          for i = from, to do out[#out + 1] = i end
          start = to + 1
        end
        return out
      end,
    }),
    attach_mappings = function(bufnr, m)
      m("i", "<C-r>", function()
        regex = not regex
        local picker = require("telescope.actions.state").get_current_picker(bufnr)
        picker.prompt_border:change_title(regex and "Find in files (regex · <C-r> as typed)"
          or "Find in files (as typed · <C-r> regex)")
        picker:refresh(finder, { reset_prompt = false })
      end)
      return true
    end,
    push_cursor_on_edit = true,
  }):find()
end

-- --- Plugins (lazy.nvim) -----------------------------------------------------

local lazypath = vim.fn.stdpath("data") .. "/lazy/lazy.nvim"
if not vim.uv.fs_stat(lazypath) then
  local out = vim.fn.system({ "git", "clone", "--filter=blob:none", "--branch=stable",
    "https://github.com/folke/lazy.nvim.git", lazypath })
  if vim.v.shell_error ~= 0 then
    vim.api.nvim_echo({ { "Couldn't clone lazy.nvim:\n" .. out, "ErrorMsg" } }, true, {})
    return
  end
end
vim.opt.rtp:prepend(lazypath)

require("lazy").setup({
  -- A dark theme.
  {
    "folke/tokyonight.nvim",
    priority = 1000,
    config = function() vim.cmd.colorscheme("tokyonight-night") end,
  },

  -- Shows the keys that follow a prefix: press <space> and wait.
  {
    "folke/which-key.nvim",
    event = "VeryLazy",
    opts = {
      icons = { mappings = false },
      spec = {
        { "<leader>g", group = "Git" },
        { "gr", group = "LSP" },
      },
    },
  },

  -- Indentation follows the file (tabs or spaces, and how many).
  "tpope/vim-sleuth",

  -- Find files, text, buffers, help: a fuzzy finder.
  {
    "nvim-telescope/telescope.nvim",
    dependencies = { "nvim-lua/plenary.nvim" },
    cmd = "Telescope",
    keys = {
      { "<leader>f", list("find_files", "gen_from_file"), desc = "Find a file" },
      { "<leader>/", find_in_files, desc = "Find in files" },
      { "<leader>b", list("buffers", "gen_from_buffer"), desc = "Open buffers" },
      { "<leader>r", list("oldfiles", "gen_from_file"), desc = "Recent files" },
      { "<leader>h", "<cmd>Telescope help_tags<CR>", desc = "Neovim's help" },
      { "<leader>d", "<cmd>Telescope diagnostics<CR>", desc = "Problems" },
      { "<leader>s", symbols, desc = "Symbols in this file" },
      { "<leader>S", "<cmd>Telescope lsp_dynamic_workspace_symbols<CR>", desc = "Symbols in the project" },
      { "<leader>gh", file_history, desc = "This file's history" },
      { "<leader>gc", list("git_status", "gen_from_git_status"), desc = "Changed files" },
    },
    opts = {},
  },

  -- Browse files as a buffer: rename by editing a line, delete with dd,
  -- then :w. Enter opens, - goes up.
  {
    "stevearc/oil.nvim",
    lazy = false, -- so `nvim .` opens it
    keys = {
      { "<leader>e", "<cmd>Oil<CR>", desc = "Browse files" },
      { "-", "<cmd>Oil<CR>", desc = "Browse the file's directory" },
    },
    opts = { columns = {}, view_options = { show_hidden = true } },
    -- nvim with no file opens this directory, as nvim . does.
    init = function()
      local stdin = false
      vim.api.nvim_create_autocmd("StdinReadPre", { callback = function() stdin = true end })
      vim.api.nvim_create_autocmd("VimEnter", {
        callback = function()
          if vim.fn.argc() == 0 and not stdin and #vim.api.nvim_list_uis() > 0
            and vim.api.nvim_buf_get_name(0) == "" and vim.fn.line2byte("$") == -1 then
            vim.schedule(function() vim.cmd.edit(".") end)
          end
        end,
      })
    end,
  },

  -- Changed lines in the sign column, and hunk actions.
  {
    "lewis6991/gitsigns.nvim",
    event = { "BufReadPre", "BufNewFile" },
    opts = {
      on_attach = function(buf)
        local gs = require("gitsigns")
        local function m(l, f, d) map("n", l, f, { buffer = buf, desc = d }) end
        m("]h", function() gs.nav_hunk("next") end, "Next change")
        m("[h", function() gs.nav_hunk("prev") end, "Previous change")
        m("<leader>gp", gs.preview_hunk, "Preview the change")
        m("<leader>gu", gs.reset_hunk, "Undo the change")
        m("<leader>gs", gs.stage_hunk, "Stage the change")
        -- The file beside the last commit's, the changes lined up;
        -- <space>gd again closes it.
        m("<leader>gd", function()
          for _, w in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
            if vim.api.nvim_buf_get_name(vim.api.nvim_win_get_buf(w)):match("^gitsigns://") then
              vim.api.nvim_win_close(w, true)
              return vim.cmd("diffoff!")
            end
          end
          gs.diffthis("HEAD")
        end, "Diff with the last commit")
        m("<leader>gb", gs.blame_line, "Blame the line")
        -- Who changed each line, in a column beside the file (IntelliJ's
        -- Annotate); <space>gB again or q closes it.
        m("<leader>gB", function()
          for _, w in ipairs(vim.api.nvim_tabpage_list_wins(0)) do
            if vim.bo[vim.api.nvim_win_get_buf(w)].filetype == "gitsigns-blame" then
              return vim.api.nvim_win_close(w, true)
            end
          end
          gs.blame()
        end, "Blame the file")
      end,
    },
    init = function()
      vim.api.nvim_create_autocmd("FileType", {
        pattern = "gitsigns-blame",
        callback = function(ev)
          map("n", "q", "<cmd>close<CR>", { buffer = ev.buf, desc = "Close the blame" })
          map("n", "<leader>gB", "<cmd>close<CR>", { buffer = ev.buf, desc = "Close the blame" })
        end,
      })
    end,
  },

  -- A link to the line, or the selected lines, where the repo is hosted
  -- (GitHub, GitLab, Bitbucket, Codeberg, self-hosted ones too): at the
  -- last commit pushed, so it opens for whoever you send it to.
  {
    "linrongbin16/gitlinker.nvim",
    cmd = "GitLink",
    keys = {
      { "<leader>gy", "<cmd>GitLink<CR>", mode = { "n", "x" }, desc = "Copy the link to the line" },
      { "<leader>go", "<cmd>GitLink!<CR>", mode = { "n", "x" }, desc = "Open the line in the browser" },
    },
    opts = {},
  },

  -- Replace in files, as IntelliJ's: what to find, what to put instead,
  -- and every change shown before you make it.
  {
    "MagicDuck/grug-far.nvim",
    cmd = "GrugFar",
    keys = {
      { "<leader>R", function() require("grug-far").open() end, desc = "Replace in files" },
    },
    opts = { icons = { enabled = false } },
  },

  -- A status line, more text objects (ci" in an argument, da( ...), and
  -- closing brackets and quotes as you open them: type the closer to step
  -- over it, <BS> deletes both, <CR> between {} opens a block.
  {
    "nvim-mini/mini.nvim",
    version = false,
    config = function()
      require("mini.statusline").setup({ use_icons = false })
      require("mini.ai").setup()
      require("mini.pairs").setup()
    end,
  },

  -- Language servers (see the top of the file). YAML and JSON know the
  -- schemas of Kubernetes, GitHub Actions, Compose and many more.
  {
    "mason-org/mason-lspconfig.nvim",
    dependencies = {
      { "mason-org/mason.nvim", opts = {} },
      "neovim/nvim-lspconfig",
      "b0o/SchemaStore.nvim",
    },
    config = function()
      local schemas = require("schemastore")
      vim.lsp.config("yamlls", {
        settings = { yaml = { schemaStore = { enable = false, url = "" }, schemas = schemas.yaml.schemas() } },
      })
      vim.lsp.config("jsonls", {
        settings = { json = { schemas = schemas.json.schemas(), validate = { enable = true } } },
      })
      -- terraform-ls keeps nothing between starts: it reads the repo again
      -- each time, and gd and grr wait for it. Two things made that most
      -- of a minute in a big repo: its deeper checks (a var or attribute
      -- that doesn't exist; syntax errors still show without them) and
      -- asking registry.terraform.io about every registry module (docs
      -- for their inputs; a module `terraform init` fetched is read from
      -- .terraform either way), which can't be turned off but fails at
      -- once through a proxy that isn't there. Without them: seconds.
      -- Back on, in local.lua: vim.lsp.config("terraformls", {
      --   init_options = { validation = { enableEnhancedValidation = true } },
      --   cmd_env = { HTTPS_PROXY = vim.env.HTTPS_PROXY or "" } })
      vim.lsp.config("terraformls", {
        init_options = { validation = { enableEnhancedValidation = false } },
        cmd_env = { HTTPS_PROXY = "http://127.0.0.1:1" },
      })
      -- Mason's servers start by themselves; a language added later gets
      -- its server installed in the background on the next start.
      local want = {}
      for _, s in ipairs(servers) do
        if not s[3] or vim.fn.executable(s[3]) == 1 then table.insert(want, s[1]) end
      end
      local headless = #vim.api.nvim_list_uis() == 0 -- yardo installs then
      require("mason-lspconfig").setup({ ensure_installed = headless and {} or want })
      for name, bin in pairs(own) do
        if vim.fn.executable(bin) == 1 then vim.lsp.enable(name) end
      end
    end,
  },

  -- Completion as you type, from the language server, the buffer and
  -- paths: up/down (or <C-n>/<C-p>) pick, <CR> accepts what you picked,
  -- <C-space> shows the docs. Nothing is picked until you move, so <CR>
  -- at the end of a line is still a new line.
  {
    "saghen/blink.cmp",
    version = "1.*",
    event = "InsertEnter",
    opts = {
      keymap = { preset = "enter" },
      completion = { list = { selection = { preselect = false } } },
      fuzzy = { implementation = "lua" },
    },
  },
}, {
  ui = { icons = { cmd = "", config = "", event = "", ft = "", init = "", keys = "", plugin = "", runtime = "", require = "", source = "", start = "", task = "", lazy = "" } },
  rocks = { enabled = false },
  change_detection = { notify = false },
})

-- Your own settings, read last so they win.
local mine = vim.fn.stdpath("config") .. "/local.lua"
if vim.uv.fs_stat(mine) then dofile(mine) end
