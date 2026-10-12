# vscode-web (fetched, not vendored)

The in-browser workspace editor uses the public
[`vscode-web`](https://www.npmjs.com/package/vscode-web) Code-OSS build (MIT).
It is ~60 MB, so it is not committed. Fetch it before building a release binary:

```bash
make web-deps
```

Without it, AgentFlow still runs; only the "open workspace in browser editor"
view is unavailable.
