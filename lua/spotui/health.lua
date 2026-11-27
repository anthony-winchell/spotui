-- :checkhealth spotui
local M = {}

function M.check()
  vim.health.start("spotui")

  if vim.fn.has("nvim-0.11") == 1 then
    vim.health.ok("Neovim " .. tostring(vim.version()))
  else
    vim.health.error("spotui needs Neovim 0.11 or newer")
  end

  local cmd = require("spotui.config").options.cmd
  local path = vim.fn.exepath(cmd)
  if path == "" then
    vim.health.error(("`%s` isn't on PATH"):format(cmd), {
      "Run `go install ./cmd/spotui` in the spotui repo",
      "Or set `cmd` in setup() to the binary's full path",
    })
    return
  end
  vim.health.ok("Found " .. path)

  local res = vim.system({ path, "status", "--json" }, { text = true }):wait(15000)
  if res.code == 0 then
    vim.health.ok("Signed in and reached Spotify")
  else
    local msg = vim.trim(res.stderr or ""):gsub("^spotui: ", "")
    vim.health.error(msg ~= "" and msg or "status check failed", {
      "Not signed in? Run :Spotui to sign in through the browser",
      "No client ID? Neovim runs spotui outside the repo, so .env isn't read: put "
        .. "`client_id = \"...\"` in spotui's config.toml (on Windows, %AppData%\\spotui\\config.toml)",
    })
  end
end

return M
