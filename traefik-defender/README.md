# Traefik Defender Plugin

The **Traefik Defender** plugin is a middleware for Traefik that allows you to block or manipulate requests based on the client's IP address. It is particularly useful for preventing unwanted traffic or polluting AI training data by returning garbage responses.

This is a companion plugin to [Caddy Defender](https://github.com/JasonLovesDoggo/caddy-defender), bringing the same functionality to the Traefik ecosystem.

## Features

- **IP Range Filtering**: Block or manipulate requests from specific IP ranges.
- **Embedded IP Ranges**: Predefined IP ranges for popular AI services (e.g., OpenAI, DeepSeek, GitHub Copilot).
- **Custom IP Ranges**: Add your own IP ranges via YAML configuration.
- **Multiple Responder Backends**:
  - **Block**: Return a `403 Forbidden` response.
  - **Custom**: Return a custom message.
  - **Drop**: Drops the connection.
  - **Garbage**: Return garbage data to pollute AI training.
  - **Redirect**: Return a `308 Permanent Redirect` response with a custom URL.
  - **Ratelimit**: Ratelimit requests.
  - **Tarpit**: Stream data at a slow, but configurable rate to stall bots and pollute AI training.

## Installation

### Local Plugin (Development)

For local development and testing, you can use Traefik's local plugin feature:

1. Clone this repository
2. Configure Traefik to use the local plugin:

```yaml
# traefik.yml
experimental:
  localPlugins:
    traefik-defender:
      moduleName: pkg.jsn.cam/caddy-defender/traefik-defender
```

3. Add the plugin to your dynamic configuration (see Configuration section below)

### Plugin Catalog (Production)

For production use, this plugin can be published to the Traefik Plugin Catalog. Instructions coming soon!

## Configuration

### Static Configuration

Enable the plugin in your Traefik static configuration:

```yaml
# traefik.yml
experimental:
  plugins:
    traefik-defender:
      moduleName: pkg.jsn.cam/caddy-defender/traefik-defender
      version: v0.1.0
```

### Dynamic Configuration

Configure the middleware in your dynamic configuration:

```yaml
# Block requests from AI services (default behavior)
http:
  middlewares:
    defender-block:
      plugin:
        traefik-defender:
          ipRanges:
            - openai
            - deepseek
            - aws
            - gcloud
          responder: block

  routers:
    my-router:
      rule: "Host(`example.com`)"
      service: my-service
      middlewares:
        - defender-block
```

#### Custom Response Example

```yaml
http:
  middlewares:
    defender-custom:
      plugin:
        traefik-defender:
          ipRanges:
            - openai
          responder: custom
          message: "AI scraping is not allowed"
          statusCode: 403
```

#### Tarpit Example

```yaml
http:
  middlewares:
    defender-tarpit:
      plugin:
        traefik-defender:
          ipRanges:
            - openai
            - deepseek
          responder: tarpit
          tarpitConfig:
            timeout: "60s"
            bytesPerSecond: 10
            responseCode: 200
```

#### Redirect Example

```yaml
http:
  middlewares:
    defender-redirect:
      plugin:
        traefik-defender:
          ipRanges:
            - openai
          responder: redirect
          url: "https://example.com/ai-policy"
```

#### With Whitelist

```yaml
http:
  middlewares:
    defender-whitelist:
      plugin:
        traefik-defender:
          ipRanges:
            - openai
            - aws
          responder: block
          whitelist:
            - "1.2.3.4"
            - "5.6.7.8"
```

## Configuration Options

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `ipRanges` | `[]string` | IP ranges to block (CIDR or predefined keys) | `["aws", "gcloud", "azurepubliccloud", "openai", "deepseek", "githubcopilot"]` |
| `responder` | `string` | Response strategy: `block`, `custom`, `drop`, `garbage`, `redirect`, `tarpit`, `ratelimit` | `"block"` |
| `message` | `string` | Custom message (for `custom` responder) | `""` |
| `statusCode` | `int` | HTTP status code (for `custom` responder) | `200` |
| `url` | `string` | Redirect URL (for `redirect` responder) | `""` |
| `whitelist` | `[]string` | IP addresses to exclude from blocking | `[]` |
| `tarpitConfig` | `object` | Configuration for tarpit responder | See below |
| `serveIgnore` | `bool` | Serve robots.txt with Disallow: / | `false` |

### Tarpit Configuration

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `timeout` | `string` | Duration to keep connection open | `"30s"` |
| `bytesPerSecond` | `int` | Rate to stream data | `24` |
| `responseCode` | `int` | HTTP status code | `200` |
| `headers` | `map[string]string` | Custom headers | `{}` |
| `content` | `string` | Content source (e.g., `file:///path` or `http://url`) | `""` |

## Predefined IP Ranges

The plugin includes predefined IP ranges for popular AI services:

| Service | Key | Description |
|---------|-----|-------------|
| Alibaba Cloud | `aliyun` | Alibaba Cloud IP ranges |
| VPNs | `vpn` | Common VPN service ranges |
| AWS | `aws` | Amazon Web Services |
| AWS Region | `aws-us-east-1`, `aws-us-west-1`, etc. | Specific AWS regions |
| DeepSeek | `deepseek` | DeepSeek AI service |
| GitHub Copilot | `githubcopilot` | GitHub Copilot service |
| Google Cloud | `gcloud` | Google Cloud Platform |
| Oracle Cloud | `oci` | Oracle Cloud Infrastructure |
| Microsoft Azure | `azurepubliccloud` | Microsoft Azure public cloud |
| OpenAI | `openai` | OpenAI services |
| Mistral | `mistral` | Mistral AI |
| Vultr | `vultr` | Vultr cloud |
| Cloudflare | `cloudflare` | Cloudflare |
| Digital Ocean | `digitalocean` | Digital Ocean |
| Linode | `linode` | Linode cloud |
| Private | `private` | Private IP ranges (RFC 1918) |
| All | `all` | All IP addresses |

## Docker Compose Example

```yaml
version: '3.7'

services:
  traefik:
    image: traefik:latest
    command:
      - "--api.insecure=true"
      - "--providers.docker=true"
      - "--entrypoints.web.address=:80"
      - "--experimental.plugins.traefik-defender.modulename=pkg.jsn.cam/caddy-defender/traefik-defender"
      - "--experimental.plugins.traefik-defender.version=v0.1.0"
    ports:
      - "80:80"
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock

  whoami:
    image: traefik/whoami
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.whoami.rule=Host(`whoami.localhost`)"
      - "traefik.http.routers.whoami.middlewares=defender@docker"
      - "traefik.http.middlewares.defender.plugin.traefik-defender.ipRanges=openai,deepseek"
      - "traefik.http.middlewares.defender.plugin.traefik-defender.responder=block"
```

## Testing

You can test the plugin locally:

```bash
# Build and test
cd traefik-defender
go test -v

# Test with a local Traefik instance
# (see Docker Compose example above)
```

## Contributing

We welcome contributions! See the main [CONTRIBUTING.md](../CONTRIBUTING.md) for guidelines.

## License

This project is licensed under the **MIT License**. See the [LICENSE](../LICENSE) file for details.

## Related Projects

- [Caddy Defender](https://github.com/JasonLovesDoggo/caddy-defender) - The original Caddy middleware version
- [Traefik Plugin Documentation](https://doc.traefik.io/traefik-hub/api-gateway/guides/plugin-development-guide)

## Acknowledgments

- Built with ❤️ using [Traefik](https://traefik.io)
- Shares core logic with [Caddy Defender](https://github.com/JasonLovesDoggo/caddy-defender)
- Uses [BART](https://github.com/gaissmai/bart) for efficient IP matching
