-- Runs spotui commands in the background, so Neovim never waits on Spotify
local config = require("spotui.config")

local M = {}

---@param msg string
---@param level? integer
function M.notify(msg, level)
  vim.notify(msg, level or vim.log.levels.INFO, { title = "spotui" })
end

---Shows a command's error, or its output if it printed any
---@param err string?
---@param out string
function M.report(err, out)
  if err then
    M.notify(err, vim.log.levels.ERROR)
  elseif out ~= "" then
    M.notify(out)
  end
end

---Runs `spotui <args>` without blocking. cb gets an error message (nil on
---success) and the trimmed stdout, on the main loop. Without cb, the result is
---shown with vim.notify.
---@param args string[]
---@param cb? fun(err: string?, out: string)
function M.run(args, cb)
  cb = cb or M.report
  local cmd = vim.list_extend({ config.options.cmd }, args)

  local ok, err = pcall(vim.system, cmd, { text = true }, function(res)
    vim.schedule(function()
      local out = vim.trim(res.stdout or "")
      if res.code == 0 then
        return cb(nil, out)
      end

      local msg = vim.trim(res.stderr or ""):gsub("^spotui: ", "")
      if msg == "" then
        msg = "spotui exited with code " .. res.code
      end
      cb(msg, out)
    end)
  end)

  -- vim.system fails right away, rather than calling back, if it can't start
  -- the program at all
  if not ok then
    cb(("can't run %s: %s (see :checkhealth spotui)"):format(config.options.cmd, err), "")
  end
end

---Runs a command that prints JSON, and decodes it
---@param args string[]
---@param cb fun(err: string?, data: any)
function M.json(args, cb)
  M.run(vim.list_extend({ "--json" }, args), function(err, out)
    if err then
      return cb(err, nil)
    end
    local ok, data = pcall(vim.json.decode, out)
    if not ok then
      return cb("unexpected output from spotui: " .. tostring(data), nil)
    end
    cb(nil, data)
  end)
end

return M
