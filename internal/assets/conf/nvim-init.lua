-- yardo's Neovim: comfortable from the first start, without knowing the
-- plugins. Press <space> and wait: which-key lists what's there.
--
--   <space>f  find a file        <space>/  search text in the project
--   <space>b  open buffers       <space>e  browse files (edit it like text)
--   <space>h  search the help    <space>?  every key map
--   <space>y  copy to your computer's clipboard (works over SSH)
--   <space>m  install language servers: pyright, ruff, gopls, jdtls,
--             yamlls, terraformls, bashls, ... (used from the next start)
--   With one: K docs · gd definition · grr references · grn rename ·
--             gra fix · <space>d problems. Go is formatted on save.
--   gcc       comment a line     <Esc><Esc>  leave a :terminal
--   Completion: <C-n>/<C-p> to move, <C-y> to accept.
--   ( [ { " ' ` close themselves (no thanks: vim.g.minipairs_disable = true
--   in local.lua).
--
-- yardo rewrites this file on a re-run. Your own settings go in
-- ~/.config/nvim/local.lua: yardo never writes it, and it's read last.

-- --- Options -----------------------------------------------------------------

vim.g.mapleader = " "
vim.g.maplocalleader = " "

local o = vim.opt
o.number = true
o.mouse = "a" -- in tmux or a terminal, hold shift to select text
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
map("n", "<leader>w", "<cmd>write<CR>", { desc = "Save" })
-- Yanks stay in Neovim; <space>y copies to your clipboard (OSC 52 over SSH).
map({ "n", "x" }, "<leader>y", '"+y', { desc = "Copy to the clipboard" })
map("n", "<leader>Y", '"+y$', { desc = "Copy to the end of the line to the clipboard" })
map("t", "<Esc><Esc>", "<C-\\><C-n>", { desc = "Leave terminal mode" })
map("n", "<leader>m", "<cmd>Mason<CR>", { desc = "Install language servers (Mason)" })

vim.diagnostic.config({ severity_sort = true, virtual_text = true, float = { border = "rounded" } })

-- Flash what was yanked.
vim.api.nvim_create_autocmd("TextYankPost", {
  callback = function() vim.hl.on_yank() end,
})

-- With a language server: gd goes to the definition (K, grr, grn, gra
-- are Neovim's own), and Go is formatted, imports included, on save.
vim.api.nvim_create_autocmd("LspAttach", {
  callback = function(ev)
    map("n", "gd", vim.lsp.buf.definition, { buffer = ev.buf, desc = "Go to the definition" })
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
    keys = {
      { "<leader>?", function() require("which-key").show({ global = true }) end, desc = "Every key map" },
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
      { "<leader>f", "<cmd>Telescope find_files<CR>", desc = "Find a file" },
      { "<leader>/", "<cmd>Telescope live_grep<CR>", desc = "Search text (ripgrep)" },
      { "<leader>b", "<cmd>Telescope buffers<CR>", desc = "Open buffers" },
      { "<leader>r", "<cmd>Telescope oldfiles<CR>", desc = "Recent files" },
      { "<leader>h", "<cmd>Telescope help_tags<CR>", desc = "Search the help" },
      { "<leader>k", "<cmd>Telescope keymaps<CR>", desc = "Search the key maps" },
      { "<leader>d", "<cmd>Telescope diagnostics<CR>", desc = "Problems" },
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
        m("<leader>gb", gs.blame_line, "Blame the line")
      end,
    },
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

  -- Language servers: <space>m (:Mason) installs one, and it starts from
  -- the next start, for files of its language. YAML and JSON know the
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
      require("mason-lspconfig").setup()
    end,
  },

  -- Completion as you type, from the language server, the buffer and
  -- paths: <C-n>/<C-p> move, <C-y> accepts, <C-space> shows the docs.
  {
    "saghen/blink.cmp",
    version = "1.*",
    event = "InsertEnter",
    opts = {
      keymap = { preset = "default" },
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
