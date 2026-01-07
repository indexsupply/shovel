# Index Supply, Co.

Shovel v1.6 is available. Read the [1.0 announcement][1].

```
curl -LO https://indexsupply.net/bin/1.6/darwin/arm64/shovel --silent
chmod +x shovel
./shovel -version
v1.6 582d
```

To install the latest on main:

```
curl -LO https://indexsupply.net/bin/main/darwin/arm64/shovel --silent
chmod +x shovel
```

## Docker

Docker images are published to GitHub Container Registry (GHCR) for version tags.

**Pull a specific version (from git tags):**

```
docker pull ghcr.io/0xsend/shovel:1.6
```

**Run with a config file:**

```
docker run -v $(pwd)/config.json:/config.json ghcr.io/0xsend/shovel:1.6 -config /config.json
```

### Required Repository Settings

To publish images to GHCR, the GitHub Actions workflow requires these permissions:

- **Actions token permissions**: `packages: write` must be enabled in workflow permissions
- The workflow uses `GITHUB_TOKEN` for authentication (no additional secrets required for GHCR)

[Company Update][2]

[Shovel][3] Ethereum to Postgres Indexer

[1]: https://indexsupply.com/shovel/1.0
[2]: https://indexsupply.com/update-1
[3]: https://indexsupply.com/shovel
