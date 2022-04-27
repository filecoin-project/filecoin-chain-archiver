# filsnap

filsnap is a software tool for creating chain exports / snapshots using the lotus filecoin node.

## Background

Filecoin network snapshots are a segment of the Filecoin chain exported to a Content Addressable aRchives (CAR) file.
They contain a chain segment large enough to allow the Filecoin network consensus protocol to apply messages
successfully.

## Building & Dependencies

- Go 1.16 or higher

```
make all
```

## Usage

A running lotus node is required with automatic restarts and a jwt token with `admin` privileges.

Setup Daemon
```
$ while true; do lotus daemon; done
```

Create Token
```
lotus auth create-token --perm admin | tr -d '\n' > token
```

```
cat > config.toml <<EOF
[[Nodes]]
  Address = "/ip4/127.0.0.1/tcp/1234"
  TokenPath = ./token"
EOF
```

```
./filsnap nodelocker run
```

```
./filsnap create --height <height> --discard
```

## Contributing

PRs accepted.

## License

Dual-licensed under [MIT](https://github.com/travisperson/filsnap/blob/master/LICENSE-MIT) + [Apache 2.0](https://github.com/travisperson/filsnap/blob/master/LICENSE-APACHE)