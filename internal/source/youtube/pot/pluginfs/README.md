# bgutil yt-dlp PO token plugin

Python plugin from [bgutil-ytdlp-pot-provider 2.0.0](https://github.com/Brainicism/bgutil-ytdlp-pot-provider/tree/2.0.0). Licensed under GPL-3.0 (see `LICENSE` in this directory). It is a separate component: livestream-viewer copies it to `data/yt-dlp-plugins/bgutil/` at runtime (yt-dlp only loads `plugin-dirs/*/yt_dlp_plugins`) and passes `--plugin-dirs`. Without this plugin, a running PO token HTTP server is ignored.
