# Daily

Daily is a minimalist desktop application that provides a quick glance at your day's schedule. It integrates with Google
Calendar to show your upcoming events in a clean, compact interface that sits in your system tray. With Daily, you can
stay on top of your schedule without constantly checking your calendar.


# Development
## Adding icons to bundle
Run
```bash
fyne bundle -o internal/ui/bundled.go --package ui --prefix Resource --append internal/assets/icons/icon.png
```
## Releasing

1. Bump version in [FyneApp.toml](FyneApp.toml)
2. Tag commit
3. Push tag

# Credits
Calendar icons made by [Freepik](https://www.flaticon.com/authors/freepik) from [Flaticon](https://www.flaticon.com)  
