# Caddy2-zlog

## Overview

`zlog` is a log middleware for [Caddy](https://github.com/caddyserver/caddy) v2, it's based on https://github.com/rs/zerolog and https://github.com/liuzl/filestore.

## Installation

Rebuild caddy as follows:

```sh
xcaddy build --with github.com/liuzl/caddy2-zlog
```

## Caddyfile syntax

```
127.0.0.1:2021 {
    zlog {
        log_dir ./server_zerolog
        split_by hour
        hash_dir ./hashstore
    }
}
```
