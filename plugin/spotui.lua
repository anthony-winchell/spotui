-- Loaded at startup: defines :Spotui only. Everything else loads on first use.
if vim.g.loaded_spotui then
  return
end
vim.g.loaded_spotui = true

vim.api.nvim_create_user_command("Spotui", function(opts)
  require("spotui").command(opts.fargs)
end, {
  nargs = "*",
  desc = "Toggle the spotui window, or run a spotui command",
  complete = function(arglead, cmdline)
    return require("spotui").complete(arglead, cmdline)
  end,
})
