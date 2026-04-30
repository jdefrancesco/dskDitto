# dskDitto Runtime Architecture

This diagram captures the high-level runtime flow from CLI startup through duplicate detection and output modes.

```mermaid
flowchart TD
    A["CLI Entrypoint<br/>cmd/dskDitto/main.go"] --> B["Parse flags + validate inputs"]
    B --> C["Build config.Config<br/>(internal/config)"]
    B --> D["Create duplicate map<br/>dmap.NewDmap(...)"]

    C --> E["Start parallel directory walk<br/>dwalk.NewDWalker(...).Run(ctx)"]
    E --> F["Filter files + hash content<br/>dfs.NewDfile(...)"]
    F -->|send *dfs.Dfile| G["dFiles channel<br/>chan *dfs.Dfile"]
    D --> H["Main monitor loop<br/>consumes dFiles channel"]
    G --> H
    H --> I["Aggregate by digest<br/>dMap.Add(dFile)"]
    I --> J["Post-scan decision branch<br/>in main.go"]

    J --> K["Text output<br/>--text / --bullet"]
    J --> L["Structured export<br/>--csv-out / --json-out"]
    J --> M["Filesystem mutation<br/>--remove / --link"]
    J --> N{"Interactive mode?"}

    N -->|--gui| O["Raylib GUI<br/>internal/rayui"]
    N -->|default| P["Bubble Tea TUI<br/>internal/ui"]
    I --> Q["Build view model<br/>dupview.New(dMap)"]
    Q --> O
    Q --> P
```

## Legend

- `config.Config`: scan policy and filtering options derived from CLI flags.
- `chan *dfs.Dfile` (`dFiles`): streaming handoff of discovered + hashed files from walker goroutines to the main monitor loop.
- `dmap.Dmap`: in-memory digest-to-file-path index used to group duplicates and drive output operations.
- `dupview.Model`: shared UI-facing representation of duplicate groups used by both `internal/ui` (TUI) and `internal/rayui` (GUI).
