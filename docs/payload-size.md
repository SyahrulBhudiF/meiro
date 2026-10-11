# Release payload measurements

Issue [#45](https://github.com/elianiva/meiro/issues/45) separates system-media
update work from changes to shipped dependencies. This change keeps ffmpeg,
yt-dlp, QuickJS, Google Sans, and the supported package formats intact.

## Measurements depend on the platform and build

`scripts/measure-payload.py` uses Python 3's standard library. It reports exact
file sizes, yt-dlp's adjacent runtime directory, package sizes, and a lossless
gzip font candidate. Missing components appear as `missing`, not zero bytes.
The release workflow records this report in each platform's job summary.

```sh
python3 scripts/measure-payload.py build/linux-amd64
python3 scripts/measure-payload.py dist/darwin-arm64 artifacts
```

The existing Linux amd64 build on the development machine, created on
2026-10-09, has the following sizes. It predates the current one-directory
yt-dlp packaging and does not contain QuickJS. These are historical measurements,
not the sizes of a fresh release from this checkout.

| Component | Bytes |
| --- | ---: |
| App executable | 25,280,775 |
| ffmpeg | 109,525,528 |
| yt-dlp, old one-file executable | 40,446,224 |
| Debian package | 93,300,582 |
| tar.gz | 93,324,296 |
| Local Fedora RPM | 80,930,695 |
| Embedded Google Sans | 4,974,940 |
| Google Sans gzip candidate | 2,182,194 |

The gzip candidate saves 2,792,746 bytes of uncompressed embedded payload.
It does not remove glyphs or variable-font axes. Actual executable size, startup
time, peak memory, and compressed-package savings remain unmeasured. Packages
already compress the font, so the download savings can be much smaller.

## Dependency changes need reproducible candidates

The largest candidate is an audio-only ffmpeg build. No smaller binary has
been built or measured yet, so there is no supported size target. Its minimum
feature set must cover the actual player and cache contracts:

- HTTP and HTTPS, TLS certificate verification, reconnects, and seeking.
- Local-file input and pipe output.
- YouTube AAC and Opus, and every container and codec accepted by the audio cache.
- Audio resampling, stereo conversion, and signed 16-bit PCM output.
- Duration probing used by cached playback.

A candidate pipeline must pin ffmpeg and TLS dependency sources and checksums,
record configure flags, build all four release targets, and retain required
licenses and source references. The comparison must use the same app commit and
tool versions. Tests must exercise real candidate binaries for streamed and
cached playback, seeking, reconnects, and unsupported-input failures before
replacement. Maintenance includes dependency security updates, platform build
failures, and codec-coverage tests on every rebuild.

Lossless font compression costs a deterministic gzip generation step and one
bounded decompression at startup. It needs a startup and memory benchmark before
adoption. Font subsetting has a higher maintenance cost: a pinned FontTools
version, an explicit Unicode and layout-feature policy, and multilingual rendering
checks. Track titles are arbitrary catalogue text, so a subset of current UI
strings is not sufficient.

yt-dlp remains the stream resolver and the warm-next downloader. Its one-directory
build avoids extraction on each invocation, at the cost of shipping its runtime
directory. A release still defaults to `latest`; comparisons and reproducible
dependency builds must set `YT_DLP_VERSION` to an explicit release. Removing it
requires verified replacement coverage for both playback and caching, as tracked
by [#10](https://github.com/elianiva/meiro/issues/10) and
[#11](https://github.com/elianiva/meiro/issues/11).

Debian, RPM, AppImage, and tar.gz repeat payload across release artifacts. That
cost is release storage and packaging time, not four installations on one user's
machine. Removing a format trades distribution coverage for storage savings and
requires a separate product decision.
