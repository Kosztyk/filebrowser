# Erugo and native File Browser sharing

The Erugo integration is optional. When Erugo is disabled, File Browser uses
its original built-in share API and UI behavior. If the optional
`/api/share-config` capability request fails, the frontend also falls back to
the native implementation instead of leaving the Share dialog unusable.

Automatic ClamAV scans are skipped only for files strictly larger than
`1,900,000,000` bytes. Manual scan actions remain available.
