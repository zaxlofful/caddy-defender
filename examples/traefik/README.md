# Traefik Defender Examples

This directory contains example configurations for using Traefik Defender.

## Basic Configuration

### Static Configuration (traefik.yml)

```yaml
# Enable the plugin
experimental:
  plugins:
    traefik-defender:
      moduleName: pkg.jsn.cam/caddy-defender/traefik-defender
      version: v0.1.0
```

### Dynamic Configuration (config.yml)

```yaml
# Block AI services with default ranges
http:
  middlewares:
    defender-default:
      plugin:
        traefik-defender:
          ipRanges:
            - openai
            - deepseek
            - githubcopilot
            - aws
            - gcloud
          responder: block

  routers:
    my-router:
      rule: "Host(`example.com`)"
      service: my-service
      middlewares:
        - defender-default

  services:
    my-service:
      loadBalancer:
        servers:
          - url: "http://localhost:8080"
```

## Advanced Examples

### Custom Message Response

```yaml
http:
  middlewares:
    defender-custom:
      plugin:
        traefik-defender:
          ipRanges:
            - openai
            - deepseek
          responder: custom
          message: "AI scraping is not permitted on this site"
          statusCode: 403
```

### Tarpit (Slow Response)

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
            headers:
              Content-Type: "text/html"
```

### Redirect to Policy Page

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

### With IP Whitelist

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
            - "1.2.3.4"    # Trusted research IP
            - "5.6.7.8"    # Development IP
```

### Serve Blocking robots.txt

```yaml
http:
  middlewares:
    defender-robots:
      plugin:
        traefik-defender:
          ipRanges:
            - openai
            - deepseek
          responder: block
          serveIgnore: true  # Serves robots.txt with Disallow: /
```

## Docker Compose Example

```yaml
version: '3.7'

services:
  traefik:
    image: traefik:latest
    command:
      - "--api.insecure=true"
      - "--providers.docker=true"
      - "--providers.file.directory=/etc/traefik/dynamic"
      - "--entrypoints.web.address=:80"
      - "--experimental.plugins.traefik-defender.modulename=pkg.jsn.cam/caddy-defender/traefik-defender"
      - "--experimental.plugins.traefik-defender.version=v0.1.0"
    ports:
      - "80:80"
      - "8080:8080"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ./traefik-dynamic.yml:/etc/traefik/dynamic/config.yml

  whoami:
    image: traefik/whoami
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.whoami.rule=Host(`whoami.localhost`)"
      - "traefik.http.routers.whoami.middlewares=defender@docker"
      - "traefik.http.middlewares.defender.plugin.traefik-defender.ipRanges=openai,deepseek"
      - "traefik.http.middlewares.defender.plugin.traefik-defender.responder=block"
```

### Dynamic Configuration File (traefik-dynamic.yml)

```yaml
http:
  middlewares:
    defender:
      plugin:
        traefik-defender:
          ipRanges:
            - openai
            - deepseek
            - githubcopilot
          responder: block
```

## Testing Your Configuration

To test your configuration:

1. Start Traefik with the plugin enabled
2. Test from an allowed IP:
   ```bash
   curl http://localhost
   ```
   Should work normally.

3. Test from a blocked IP range (simulated):
   ```bash
   curl -H "X-Forwarded-For: 23.98.142.0" http://localhost
   ```
   Should be blocked (this is an OpenAI IP).

## Available Responders

| Responder | Description | Configuration |
|-----------|-------------|---------------|
| `block` | Return 403 Forbidden | Default, no extra config |
| `custom` | Custom message | `message`, `statusCode` |
| `drop` | Drop connection | No extra config |
| `garbage` | Random garbage data | No extra config |
| `redirect` | Permanent redirect | `url` (required) |
| `tarpit` | Slow response stream | `tarpitConfig` |
| `ratelimit` | Rate limiting | No extra config |

## Available IP Ranges

See the main [README.md](../../README.md#embedded-ip-ranges) for the full list of predefined IP ranges.

Common ones:
- `openai` - OpenAI services
- `deepseek` - DeepSeek AI
- `githubcopilot` - GitHub Copilot
- `aws` - All AWS IP ranges
- `gcloud` - Google Cloud Platform
- `azurepubliccloud` - Microsoft Azure
- `cloudflare` - Cloudflare
- `all` - All IP addresses (use with whitelist)
- `private` - Private IP ranges (RFC 1918)
