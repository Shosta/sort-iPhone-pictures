# Data sources

`world.bin.gz` is built from **Natural Earth** 1:50m vector data
(<https://www.naturalearthdata.com>), which is in the **public domain**
("Made with Natural Earth. Free vector and raster map data"):

- `ne_50m_admin_0_countries` (land and country shapes)
- `ne_50m_lakes`

Source files:
<https://github.com/nvkelso/natural-earth-vector/tree/master/geojson>

Rebuild with:

    go run ./internal/mapbase/gen ne_50m_admin_0_countries.geojson ne_50m_lakes.geojson internal/mapbase/world.bin.gz

The generator simplifies the outlines (Douglas-Peucker, 0.012°), drops tiny
islands and stores 0.01° coordinates as delta-encoded varints (about 120 KB).

## Online tile backgrounds (optional)

`mapbase.Providers` can fetch raster tiles at export/preview time (only tile coordinates are sent; never user data).
Tiles are cached on disk and fetched with an app-identifying User-Agent, a few tiles per map.
- OpenStreetMap standard: © OpenStreetMap contributors (ODbL)
- Carto Positron: © OpenStreetMap contributors © CARTO
- OpenTopoMap: © OpenStreetMap contributors, SRTM | © OpenTopoMap (CC-BY-SA)
- Esri World Imagery: Esri, Maxar, Earthstar Geographics
The attribution is drawn on the exported map. The offline vector map remains the fallback.
