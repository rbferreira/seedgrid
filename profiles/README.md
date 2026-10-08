# Site profiles

A **profile** is what SeedGrid needs to know about a tracker **as a site**, and that holds
for anyone with an account on it: the categories, types and resolutions with the IDs that
site uses, how it wants an upload described, and which image host it accepts. It is knowledge
about the site, not about the operator — which is why it can be shared.

This folder holds the profiles the community has contributed, one file per site:

```
profiles/<family>/<site>.json      e.g. profiles/unit3d/samaritano-cc.json
```

`<family>` is the tracker's API family (today, `unit3d`).

## Which sites are known to work

| site | family | status |
|---|---|---|
| samaritano | `unit3d` | tested end to end (taxonomy discovery, publishing, profile export/import) |
| Blutopia | `unit3d` | same API family — expected to work, **not tested** |
| Aither | `unit3d` | same API family — expected to work, **not tested** |

Every UNIT3D site exposes the same API, but each one configures its own categories, types and
resolutions with its own IDs — that is what a profile captures. If you use SeedGrid with a
site from this list (or any other UNIT3D site), open an issue saying whether it worked, and
contribute its profile so the next person does not start from zero.

## Using a profile

In the Portal, under **Settings → Trackers**, register the tracker with **your own**
credentials. Then, in its profile, under **Share this site's profile**, click **Import
profile** and pick the file from this folder. The Portal shows what will change before it
applies anything, and never replaces what you adjusted by hand unless you ask it to.

## Contributing your site's profile

1. Get the tracker's profile in order in your SeedGrid (taxonomy discovery and manual
   adjustments are kept in the profile).
2. In the same panel, click **Export profile**. The file comes out in the right format and
   with the right name.
3. Remove the `seedgrid-profile-<family>-` prefix from the name, put the file under
   `profiles/<family>/` and open a pull request.

Do not edit an exported file by hand: CI checks that it is exactly what the export produces.

## Writing a profile without running SeedGrid

You do not need a SeedGrid install to contribute a profile — only an account on the site and
a text editor. Start from [`TEMPLATE.json`](TEMPLATE.json): it is a complete, valid profile
with example values, in the exact form CI expects (a test keeps it that way).

1. **Copy the template** to `profiles/unit3d/<file>.json`. The file name comes from the
   site's host: lower case, every run of characters other than `a-z`/`0-9` becomes one `-`.
   `tracker.example` → `tracker-example.json`; `www.my-site.org` → `www-my-site-org.json`.
2. **Fill in `site`**: `name` is how people call the site; `host` is its address without
   `https://` or a path (`tracker.example`).
3. **Replace the taxonomy with the site's own IDs** (see below). Keep each list sorted by
   `id`.
4. **Keep or delete the optional sections** (`description`, `image_host`).
5. **Open a pull request.** CI validates the file; if all it complains about is the layout,
   the failure message prints the exact file it expects — copy it over yours.

If you have Go installed, you can run the same check locally from `control-plane/`:
`go test ./internal/trackerprofile -run TestRepoProfiles`.

### Finding the site's IDs

Every UNIT3D site numbers its categories, types and resolutions its own way — `Encode` may be
3 on one site and 12 on another. Copying IDs from another profile publishes into the wrong
category, so take them from **your** site, in either of two ways:

- **Upload page.** Open the site's upload form, right-click the *Category* drop-down and pick
  *Inspect*. Each `<option value="7">Movies</option>` is one entry: `value` is the `id`, the
  text is the `name`. Do the same for *Type* and *Resolution*.
- **API.** With your own API token (from your profile settings on the site), open
  `https://<site>/api/torrents/filter?api_token=<your token>`. Each torrent in the answer
  carries `category`/`category_id`, `type`/`type_id` and `resolution`/`resolution_id`.
  Never paste the URL with your token into an issue or a pull request.

### The fields

| field | what it is |
|---|---|
| `format` | always `"seedgrid.tracker-profile"` |
| `site.name`, `site.host` | the site, as above; up to 64 characters, no control characters |
| `family` | the API family; today always `"unit3d"`, and the file lives in `profiles/unit3d/` |
| `schema_version` | always `1` |
| `taxonomy.category` | the site's categories: `id` (1 to 1,000,000, unique in the list), `name` (up to 64 characters) and, optionally, `kind` |
| `taxonomy.resolution`, `taxonomy.type` | same as `category`, without `kind` |
| `description` | optional — how SeedGrid composes the upload description for this site |
| `image_host` | optional — the site's own image host for screenshots |

**`kind`** tells SeedGrid what a category is for, beyond its number: `movie` (films — the
default when publishing), `tv` (shows season and episode fields) or `game` (looks the game up
on IGDB). Leave it out for everything else (music, books, software…). Mark at least one
`movie` and one `tv` category if the site has them; publishing pre-selects them.

**`description`** — delete the section to use the default, which is what the template shows.
`media_blocks` (films and series) is an ordered list taken from `poster`, `synopsis`, `tags`,
`screenshots`, `mediainfo`; `game_blocks` from `cover`, `title`, `about`, `genres`,
`platforms`. Leave a block out to switch it off; `[]` means none. `media_info_tag` is the
BBCode tag that wraps the MediaInfo: `mediainfo`, `code`, `pre` or `spoiler` — use what the
site's upload rules ask for. `language` is the language of the content SeedGrid writes for this site: the block
headings and the synopsis and genres it fetches from TMDB — `en` or `pt-BR`.

**`image_host`** — delete the section if the site has no image host of its own: screenshots
then go to imgbb. If it has one, describe its upload API:

| field | meaning |
|---|---|
| `name` | a label for the host, shown to the operator |
| `upload_url` | where the image is POSTed: `https`, a DNS name (no IP, no port, no `localhost`) |
| `file_field` | the name of the multipart field that carries the image |
| `auth_header` | the header that carries the operator's key (e.g. `X-API-Key`, `Authorization`) |
| `auth_prefix` | text placed before the key in that header (e.g. `Bearer `); `""` for none |
| `url_path` | where the image URL is in the JSON answer, with dots (`data.url`) |
| `thumbnail_path` | the same for the thumbnail; delete it if the host has none |

The key itself never goes into the file: each operator enters their own in the Portal.

## What never goes into a profile

No credentials of any kind. The format **has no field** for an `api_token`, a passkey, an
announce URL or an image host key — those are the operator's configuration, they live
elsewhere in the database, and the export does not read them. A file with any field outside
the format is rejected both by the import and by this folder's test (`TestRepoProfiles`, in
the Control Plane).

## What CI checks

Each file goes through the same validation as an import from the Portal (format, version,
family, taxonomy), sits in the folder of the family it declares, is named after the site's
host, and is in the canonical form the export produces.
