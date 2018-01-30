# Cache

## What exactly `cache` does?

The purpose of `cache` is to create N HTTP server proxies that intercept requests, make the request to the actual server and persists both request and response info (body + headers). Then the client receives the response transparently. If somebody requests the same resource again, it will get
the `cached` response.

## Why?

With `cache`, you can make quick n' dirty integration tests. Sometimes you don't have the time to mock all your services, or you have to replicate an special scenario or play around with the header responses. `Cache` helps you to inspect your requests and responses.

## Installation

If you have a `Go` installation you can get the development version:

> go get -u github.com/thewraven/cache

the fresh binary is in your `$GOPATH/bin` folder.


If you want a ready-to-use binary, you can grab'em [here](https://github.com/thewraven/cache/releases).

## Usage

> cache -config yourservers.json

`yourservers.json` defines the proxies that will be run.

This is an example of a simple server configuration.

* Remote: the `ip:port` of the real server.
* Local: the `ip:port` of the proxy.
* Cache: if it is set to `true`, it'll reuse the latest response of the real server.
* Timeout (minutes): After `x` minutes, the latest server response will be invalidated.
* Cache_path: Folder when the requests and responses will be persisted.

You can define an array of configurations, `cache` will spawn as many servers as required.

```json
[
    {
        "remote": "yourrealserver.com:8888",
        "local": ":9999",
        "cache": true,
        "timeout_minutes": 10,
        "cache_path": "your_cache_folder"
    }
]
```
