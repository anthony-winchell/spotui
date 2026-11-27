-- The TUI in a floating terminal. Hiding the window keeps the TUI running, so
-- reopening it is instant; quitting the TUI (q) closes the window for good.
local config = require("spotui.config")

local M = {}

local state = {
  buf = nil, ---@type integer?
  win = nil, ---@type integer?
  running = false, -- the TUI process is alive
}

-- spotui's own minimum size (see minWidth/minHeight in internal/ui/view.go)
local MIN_WIDTH, MIN_HEIGHT = 50, 16

---The float's position and size, centred in the editor
local function layout()
  local o = config.options.window
  local cols = vim.o.columns
  local lines = vim.o.lines - vim.o.cmdheight

  -- The border takes one cell on each side, so leave room for it
  local width = math.min(cols - 2, math.max(MIN_WIDTH, math.floor(cols * o.width)))
  local height = math.min(lines - 2, math.max(MIN_HEIGHT, math.floor(lines * o.height)))

  return {
    relative = "editor",
    width = width,
    height = height,
    row = math.floor((lines - height) / 2),
    col = math.floor((cols - width) / 2),
    style = "minimal",
    border = o.border,
    title = " spotui ",
    title_pos = "center",
  }
end

local function visible()
  return state.win ~= nil and vim.api.nvim_win_is_valid(state.win)
end

local function wipe()
  if state.buf and vim.api.nvim_buf_is_valid(state.buf) then
    vim.api.nvim_buf_delete(state.buf, { force = true })
  end
  state.buf, state.running = nil, false
end

---Hides the window. If the TUI has exited, its buffer goes too.
function M.hide()
  if visible() then
    vim.api.nvim_win_close(state.win, true)
  end
  state.win = nil
  if not state.running then
    wipe()
  end
end

---Starts the TUI in the current window, which must show a fresh buffer
local function start()
  local buf = state.buf
  -- jobstart throws, rather than returning -1, when cmd isn't executable
  local ok, job = pcall(vim.fn.jobstart, { config.options.cmd }, {
    term = true,
    on_exit = function(_, code)
      state.running = false
      vim.schedule(function()
        if code == 0 then
          M.hide() -- quit normally: close up
        end
        -- Otherwise leave the window open so the error stays readable;
        -- the hide key closes it.
        require("spotui.statusline").refresh()
      end)
    end,
  })

  if not ok or job <= 0 then
    M.hide()
    require("spotui.cli").notify(
      ("can't run %s (see :checkhealth spotui)"):format(config.options.cmd),
      vim.log.levels.ERROR
    )
    return
  end
  state.running = true

  local hide_key = config.options.window.hide_key
  if hide_key then
    vim.keymap.set({ "t", "n" }, hide_key, M.hide, { buffer = buf, desc = "Hide spotui" })
  end

  -- Clicking or jumping back into the window should type into the TUI, not
  -- land in terminal-normal mode
  vim.api.nvim_create_autocmd("BufEnter", {
    buffer = buf,
    callback = function()
      if state.running then
        vim.cmd.startinsert()
      end
    end,
  })
end

---Shows the window, starting the TUI if it isn't running
function M.open()
  if visible() then
    vim.api.nvim_set_current_win(state.win)
  else
    local fresh = not (state.buf and vim.api.nvim_buf_is_valid(state.buf))
    if fresh then
      state.buf = vim.api.nvim_create_buf(false, true)
      vim.bo[state.buf].bufhidden = "hide" -- keep it, and the TUI, when hidden
    end

    state.win = vim.api.nvim_open_win(state.buf, true, layout())

    if fresh then
      start()
      if not visible() then
        return -- couldn't start
      end
    end
  end

  if state.running then
    vim.cmd.startinsert()
  end
end

---Hides the window if you're in it, otherwise shows it
function M.toggle()
  if visible() and vim.api.nvim_get_current_win() == state.win then
    M.hide()
  else
    M.open()
  end
end

-- Keep the float centred and sized when the editor is resized. Neovim passes
-- the new size on to the TUI, which redraws to fit.
vim.api.nvim_create_autocmd("VimResized", {
  group = vim.api.nvim_create_augroup("spotui_window", { clear = true }),
  callback = function()
    if visible() then
      vim.api.nvim_win_set_config(state.win, layout())
    end
  end,
})

return M
