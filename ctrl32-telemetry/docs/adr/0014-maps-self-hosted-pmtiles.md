# ADR-0014: Maps — self-hosted raster PMTiles, organization tile URL, OpenStreetMap only for light use

**Status:** accepted 25 Sep 2026
**Context:** Map widgets and device positions (URS J-19) need tile imagery. Since 0.16.0 an organization could set its
own XYZ tile URL, with the public OpenStreetMap tile servers as the built-in default — acceptable for light use only
under their policy and useless on an intranet without Internet access, which is the deployment the product is built
for (ADR-0003). A tile server next to the platform (tileserver-gl, Martin) would be a second service to install,
monitor and back up, against the single-binary principle.

## Decision

1. **The server serves tiles itself from one PMTiles v3 archive** (`[map] pmtiles = "…/map.pmtiles"`,
   optional `attribution`). PMTiles is a single file with an internal directory, readable by range reads; the reader
   (`internal/pmtiles`) is our own, small and dependency-free (header, root and leaf directories, Hilbert tile ids,
   gzip or no compression). Only raster archives (PNG, JPEG, WebP, AVIF) are accepted, because the map widgets use
   Leaflet with raster layers.
2. **Precedence:** an organization's own tile URL wins; with an empty URL the server's archive is used when configured;
   without either the built-in OpenStreetMap source remains, labelled as such in the settings.
3. **Tiles are public and cacheable** (`GET /tiles/{z}/{x}/{y}.png`, `Cache-Control: public, max-age=86400`): map
   imagery is not telemetry, public dashboard links need it without a session, and browsers and proxies may cache it.
   Only the archive's name, zoom range, bounds and type are exposed through `GET /api/v1/tiles`; never the file path.
4. **Vector tiles are deferred.** Vector PMTiles (MVT) need MapLibre in the browser; per ADR-0013 that arrives with
   the component UI. The reader already parses vector archives, the server refuses them with a clear message.
5. **Archives come from the operator.** Documented paths: convert an MBTiles raster export with `pmtiles convert`
   (go-pmtiles, BSD-3), or download a ready raster archive of the region; `tools/pmtilesgen` produces a synthetic
   sample for tests and demonstrations (`https://telemetry.digital/dl/sample.pmtiles`).

## Consequences

- An intranet installation gets working maps with one file on disk and one line of configuration; no extra service.
- Disk is the operator's concern: a raster archive of one country to zoom 14 is several gigabytes; the archive is a
  static asset, not a record, so it is excluded from backups and restored by copying.
- The public tiles endpoint reveals that the installation has a map of a certain region; nothing about devices.
- Brotli/zstd-compressed archives are rejected with a message asking for gzip or no compression, to keep the reader
  dependency-free; the `pmtiles` CLI can re-encode them.
