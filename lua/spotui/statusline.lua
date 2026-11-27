-- The current track for a statusline. Polls `spotui status --json` in the
-- background and advances the progress locally between polls, the same way
-- the TUI does. Nothing is polled until a statusline first asks for the text.
local cli = require("spotui.cli")
local config = require("spotui.config")

local M = {}

local state = {
  started = false,
  data = nil, ---@type table? the last status --json result
  fetched_at = 0, -- vim.uv.now() when data arrived
  inflight = false,
  again = false, -- poll again as soon as the current one finishes
  paused = false, -- Neovim doesn't have focus
  timer = nil, ---@type uv.uv_timer_t?
}

local function redraw()
  local ok, lualine = pcall(require, "lualine")
  if ok then
    lualine.refresh()
  else
    vim.cmd.redrawstatus()
  end
end

---Schedules the next poll in ms, replacing any that's waiting
local function schedule(ms)
  state.timer:stop()
  state.timer:start(ms, 0, vim.schedule_wrap(M.poll))
end

---Asks spotui for the playback state now
function M.poll()
  if state.inflight then
    state.again = true
    return
  end
  state.inflight = true

  cli.json({ "status" }, function(err, data)
    state.inflight = false
    -- Errors (not signed in, offline) just blank the statusline. Commands you
    -- run yourself still report them.
    state.data = not err and data or nil
    state.fetched_at = vim.uv.now()

    local o = config.options.statusline
    local playing = state.data and state.data.active
    if not state.paused then
      schedule(state.again and 0 or (playing and o.interval or o.idle_interval))
    end
    state.again = false
    redraw()
  end)
end

---Starts polling; the statusline functions call this themselves
function M.start()
  if state.started then
    return
  end
  state.started = true
  state.timer = vim.uv.new_timer()

  -- No point asking Spotify while you're in another window
  local group = vim.api.nvim_create_augroup("spotui_statusline", { clear = true })
  vim.api.nvim_create_autocmd("FocusLost", {
    group = group,
    callback = function()
      state.paused = true
      state.timer:stop()
    end,
  })
  vim.api.nvim_create_autocmd("FocusGained", {
    group = group,
    callback = function()
      state.paused = false
      schedule(0)
    end,
  })
  vim.api.nvim_create_autocmd("VimLeavePre", {
    group = group,
    callback = function()
      state.timer:stop()
      state.timer:close()
    end,
  })

  M.poll()
end

---Polls soon, after a command has changed playback. Spotify takes a moment to
---report the new state, so asking right away tends to get the old one.
function M.refresh()
  if state.started and not state.paused then
    schedule(400)
  end
end

---@param ms number
local function mmss(ms)
  local s = math.floor(ms / 1000)
  return ("%d:%02d"):format(math.floor(s / 60), s % 60)
end

---@param s string
---@param n integer
local function truncate(s, n)
  if vim.fn.strchars(s) <= n then
    return s
  end
  return vim.fn.strcharpart(s, 0, n - 1) .. "…"
end

---Whether there's a track to show
function M.active()
  M.start()
  local d = state.data
  return d ~= nil and d.active and d.track ~= nil
end

---The statusline text, e.g. "▶ Creep — Radiohead 1:02/3:58", or "" when
---nothing is playing
function M.text()
  if not M.active() then
    return ""
  end
  local d = state.data

  local progress = d.progress_ms
  if d.playing then
    progress = math.min(progress + (vim.uv.now() - state.fetched_at), d.duration_ms)
  end

  local name = d.track
  if d.artists and #d.artists > 0 then
    name = name .. " — " .. table.concat(d.artists, ", ")
  end
  name = truncate(name, config.options.statusline.max_width)

  local text = ("%s %s %s/%s"):format(d.playing and "▶" or "⏸", name, mmss(progress), mmss(d.duration_ms))
  return (text:gsub("%%", "%%%%")) -- a "%" in a title would be read as a statusline item
end

---The last status --json result, for building your own statusline
function M.data()
  M.start()
  return state.data
end

return M
