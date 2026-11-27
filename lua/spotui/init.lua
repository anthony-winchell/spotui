-- spotui for Neovim: the TUI in a floating window, playback keymaps and
-- commands that work without it, and a statusline component.
local config = require("spotui.config")

local M = {}

---Returns a function that runs `spotui <args>` and refreshes the statusline
---@param args string[]
local function command(args)
  return function()
    require("spotui.pickers").play(args)
  end
end

---Everything the keymaps can do, by the names used in config.keys
M.actions = {
  toggle = function()
    require("spotui.window").toggle()
  end,
  play_pause = command({ "toggle" }),
  next = command({ "next" }),
  prev = command({ "prev" }),
  like = command({ "togglelike" }),
  shuffle = command({ "shuffle" }),
  cycle_repeat = command({ "repeat" }),
  vol_up = function()
    command({ "vol", "+" .. config.options.volume_step })()
  end,
  vol_down = function()
    command({ "vol", "-" .. config.options.volume_step })()
  end,
  seek_forward = function()
    command({ "seek", "+" .. config.options.seek_step })()
  end,
  seek_back = function()
    command({ "seek", "-" .. config.options.seek_step })()
  end,
  search = function()
    require("spotui.pickers").search()
  end,
  liked = function()
    require("spotui.pickers").liked()
  end,
  playlists = function()
    require("spotui.pickers").playlists()
  end,
  status = command({ "status" }),
}

local descriptions = {
  toggle = "Toggle spotui",
  play_pause = "Play/pause",
  next = "Next track",
  prev = "Previous track",
  like = "Like/unlike track",
  shuffle = "Toggle shuffle",
  cycle_repeat = "Cycle repeat",
  vol_up = "Volume up",
  vol_down = "Volume down",
  seek_forward = "Seek forward",
  seek_back = "Seek back",
  search = "Search and play",
  liked = "Play from Liked Songs",
  playlists = "Play a playlist",
  status = "What's playing",
}

---:Spotui subcommands that do more than pass their arguments to the CLI
local subcommands = {
  search = function(args)
    require("spotui.pickers").search(table.concat(args, " "))
  end,
  liked = function(args)
    if #args == 0 then
      return require("spotui.pickers").liked()
    end
    command(vim.list_extend({ "liked" }, args))() -- :Spotui liked 5
  end,
  playlists = function()
    require("spotui.pickers").playlists()
  end,
}

---Names offered when completing :Spotui
local completions = {
  "toggle", "play", "pause", "next", "prev", "seek", "vol", "shuffle", "repeat",
  "like", "unlike", "togglelike", "search", "liked", "playlists", "recent", "status",
}

---Runs :Spotui. With no arguments it toggles the window; anything else is a
---spotui command, e.g. `:Spotui next` or `:Spotui vol +10`.
---@param fargs string[]
function M.command(fargs)
  if #fargs == 0 then
    return M.actions.toggle()
  end

  local sub = subcommands[fargs[1]]
  if sub then
    return sub(vim.list_slice(fargs, 2))
  end
  command(fargs)()
end

---@param arglead string
---@param cmdline string
function M.complete(arglead, cmdline)
  -- Only the first argument is a command name
  if #vim.split(vim.trim(cmdline), "%s+") > (arglead == "" and 1 or 2) then
    return {}
  end
  return vim.tbl_filter(function(name)
    return vim.startswith(name, arglead)
  end, completions)
end

---A lualine component: add `require("spotui").lualine()` to a section
function M.lualine()
  local statusline = require("spotui.statusline")
  return {
    statusline.text,
    cond = statusline.active,
    on_click = function()
      M.actions.toggle()
    end,
  }
end

---@param opts? table see spotui.config
function M.setup(opts)
  config.setup(opts)

  local keys = config.options.keys
  if not keys then
    return
  end

  for name, lhs in pairs(keys) do
    local action = M.actions[name]
    if lhs and action then
      vim.keymap.set("n", lhs, action, { desc = descriptions[name] })
    end
  end

  -- Name the group in which-key's popup, if you have it
  local ok, wk = pcall(require, "which-key")
  if ok and keys.toggle and keys.toggle:match("^<leader>m") then
    wk.add({ { "<leader>m", group = "music" } })
  end
end

return M
