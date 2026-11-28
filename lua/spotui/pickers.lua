-- Pick something to play with vim.ui.select. In LazyVim that's the snacks
-- picker, so you can fuzzy-filter the list.
local cli = require("spotui.cli")

local M = {}

---Runs a playback command, then updates the statusline
---@param args string[]
local function play(args)
  cli.run(args, function(err, out)
    cli.report(err, out)
    if not err then
      require("spotui.statusline").refresh()
    end
  end)
end

---@param items {kind: string, name: string, uri: string}[]
---@param prompt string
---@param on_choice fun(item: table, idx: integer)
local function pick(items, prompt, on_choice)
  if #items == 0 then
    return cli.notify("Nothing found")
  end
  vim.ui.select(items, {
    prompt = prompt,
    format_item = function(item)
      return item.kind == "track" and item.name or ("%s  (%s)"):format(item.name, item.kind)
    end,
  }, function(item, idx)
    if item then
      on_choice(item, idx)
    end
  end)
end

---Searches Spotify, then plays the chosen track, artist or album. Asks for
---the query if there isn't one.
---@param query? string
function M.search(query)
  if not query or query == "" then
    vim.ui.input({ prompt = "Search Spotify: " }, function(input)
      if input and input ~= "" then
        M.search(input)
      end
    end)
    return
  end

  cli.json({ "search", query }, function(err, items)
    if err then
      return cli.report(err, "")
    end
    pick(items, "Play: " .. query, function(item)
      M.play_item(item)
    end)
  end)
end

---Plays from the chosen liked song onwards, through the rest of the library
function M.liked()
  cli.json({ "liked" }, function(err, items)
    if err then
      return cli.report(err, "")
    end
    pick(items, "Liked Songs", function(_, idx)
      play({ "liked", tostring(idx) })
    end)
  end)
end

---Plays the chosen playlist
function M.playlists()
  cli.json({ "playlists" }, function(err, items)
    if err then
      return cli.report(err, "")
    end
    pick(items, "Playlists", function(item)
      play({ "play-uri", item.uri })
    end)
  end)
end

---Plays from the chosen recently played track, carrying on through the album
---or playlist it was played from
function M.recent()
  cli.json({ "recent" }, function(err, items)
    if err then
      return cli.report(err, "")
    end
    pick(items, "Recently Played", function(item)
      M.play_item(item)
    end)
  end)
end

---Plays an item from spotui's JSON output. A track with a context plays
---within it, so playback carries on through the album or playlist.
---@param item {uri: string, context?: string}
function M.play_item(item)
  local args = { "play-uri", item.uri }
  if item.context then
    table.insert(args, item.context)
  end
  play(args)
end

M.play = play

return M
