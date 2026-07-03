# Third-Party Runtime Assets

The `sboxkit` release artifacts aggregate this project's manager and the
upstream sing-box core as two independent executables. The sing-box source is not
linked into the `sboxkit` binary.

## Included In Release Artifacts

- `sboxkit`: MIT-licensed manager built from this repository.
- embedded WebUI: MIT-licensed static assets maintained in this repository.
- minimal bootstrap rules: MIT-licensed package data maintained in this
  repository.
- `sing-box`: upstream executable from SagerNet/sing-box release assets,
  installed separately as `/usr/lib/sboxkit/sing-box` in the Debian package
  or included next to `sboxkit` in the portable bundle.

The source repository does not commit the sing-box binary. Release automation
downloads it from upstream and records its version/source URL in
`SING_BOX_SOURCE.txt`.

## Not Included

- SagerNet/sing-geosite large rule sets
- SagerNet/sing-geoip large rule sets
- subscription data, node credentials, or generated user configuration
- subconverter software or public subconverter service code

## Runtime Downloads

Commands such as `sboxkit init --download` and `sboxkit update` may download
additional rule-set assets after installation. First-run configs do not require
large rule-set assets; after the proxy service is running, users can download
those assets through the local proxy with `sboxkit update --proxy
http://127.0.0.1:7890 --sync-service`. Downloaded files are separate upstream
works and remain governed by their upstream licenses and terms.

Known upstream sources:

| Asset | Source | Packaging status |
| --- | --- | --- |
| sing-box core | https://github.com/SagerNet/sing-box | Bundled as separate executable |
| Large GeoSite rule sets | https://github.com/SagerNet/sing-geosite | Not bundled |
| Large GeoIP rule sets | https://github.com/SagerNet/sing-geoip | Not bundled |

The sing-box project states GPL-3.0-or-later licensing upstream. Users and
redistributors must review each upstream project's license before redistributing
downloaded assets.

## Package Copyright File

The Debian package installs a copyright notice at:

```text
/usr/share/doc/sboxkit/copyright
```

That file records the package's MIT license and the explicit non-distribution
status of runtime assets.
