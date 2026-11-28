local M = {}

---@class spotui.Config
M.defaults = {
  -- The spotui binary: a name on PATH (`go install ./cmd/spotui`) or a full path
  cmd = "spotui",

  -- The floating window the TUI runs in
  window = {
    width = 0.85, -- fraction of the editor
    height = 0.85,
    border = "rounded",
    hide_key = "<C-q>", -- hides the window from inside it; the TUI keeps running
  },

  -- Global keymaps. Set the whole table to false to skip them, or one to false
  -- to skip just that one.
  keys = {
    toggle = "<leader>mm",
    play_pause = "<leader>mp",
    next = "<leader>mn",
    prev = "<leader>mN",
    like = "<leader>ml",
    shuffle = "<leader>ms",
    cycle_repeat = "<leader>mr",
    vol_up = "<leader>m+",
    vol_down = "<leader>m-",
    seek_forward = "<leader>m.",
    seek_back = "<leader>m,",
    search = "<leader>m/",
    liked = "<leader>mL",
    playlists = "<leader>mP",
    recent = "<leader>mR",
    status = "<leader>mi",
  },

  volume_step = 10, -- percent
  seek_step = 15, -- seconds

  statusline = {
    interval = 5000, -- ms between polls while something is playing
    idle_interval = 30000, -- ms between polls with no device or after an error
    max_width = 40, -- characters of "title — artist" before it's cut off
    show_liked = true, -- a ♥ after songs in your Liked Songs
  },
}

---@type spotui.Config
M.options = vim.deepcopy(M.defaults)

---@param opts? table
function M.setup(opts)
  M.options = vim.tbl_deep_extend("force", vim.deepcopy(M.defaults), opts or {})
  if opts and opts.keys == false then
    M.options.keys = false
  end
end

return M
