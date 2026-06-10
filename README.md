# unicast-mpv
> A simple module that exposes [MPV](http://mpv.io/) to be controlled through a [RPC WebSockets](https://github.com/elpheria/rpc-websockets) API, that can be used with the [unicast](https://github.com/pedromsilvapt/unicast) server

## Installation

### From .deb package (Debian/Ubuntu)

Install from the Gitea Debian package registry:

```shell
curl -fsSL https://gitea.home/api/packages/Silvas/debian/repository.key \
  | sudo tee /etc/apt/trusted.gpg.d/gitea-unicast-mpv.asc
echo "deb https://gitea.home/api/packages/Silvas/debian stable main" \
  | sudo tee /etc/apt/sources.list.d/gitea-unicast-mpv.list
sudo apt update && sudo apt install unicast-mpv
```

Or download the `.deb` for your architecture and install manually:

```shell
sudo dpkg -i unicast-mpv_<version>_linux_<arch>.deb
```

### From source

```shell
make build
sudo cp bin/unicast-mpv /usr/local/bin/
```

## Usage
Simply execute the application. The path to an YAML config file can optionally be passed as an argument. By default, the server listens on the port `2019`.
```shell
# With only default configuration
unicast-mpv
# Or with custom configuration
unicast-mpv path/to/configuration.yaml
```

## Configuration
Below is the default configuration.
```yaml
player:
    # Launch the player window in full screen
    fullscreen: true

    # Index of the monitor (0-32) where to open the player window
    monitor: null

    # Force the player window to always be on top
    onTop: false

    # If false, when stopping a video, the player window is kept open. If true, it is automatically closed
    quitOnStop: true

    # Determines if the player restarts everytime the user plays a new media when something was already playing
    restartOnPlay: false

server:
    # The network port where the socket server will listen for incoming connections
    port: 2019

    # The network interface the server will bind to
    address: 0.0.0.0

    # Specify a password to authenticate clients. Null means no password
    authenticate: null

    # MPV provides a command that allows to run arbitrary system commands. In unprotected environments, 
    # this can present a security risk. Setting this value to true disables the command from socket requests
    disableRunCommand: false
```

On Windows, the following configuration file is also loaded. If the MPV binary is in a different folder, then it should be changed to reflect that.
```yaml
player:
    binary: C:\Program Files\mpv\mpv.exe
```

## Release Pipeline

`.deb` packages for **linux/amd64** and **linux/arm64** are built with [GoReleaser](https://goreleaser.com/).

### Local dry-run (no upload, no git tag required)

```shell
make snapshot
```

This builds the binary and `.deb` packages into `dist/` without publishing anything.

### Local release (requires a git tag, publishes .deb to Gitea)

```shell
git tag v0.1.0
make release GITEA_USERNAME=myuser GITEA_PASSWORD=mytoken
```

This builds `.deb` packages for both architectures and uploads them to the Gitea Debian registry.

### Overridable package metadata

| Variable                 | Default                                            |
|--------------------------|----------------------------------------------------|
| `DEB_MAINTAINER`         | `Pedro Silva <pemiolsi@hotmail.com>`               |
| `DEB_VENDOR`             | `Pedro Silva`                                      |
| `DEB_HOMEPAGE`           | `https://github.com/pedromsilvapt/unicast-mpv`     |
| `GITEA_USERNAME`         | *(empty — must be set for `make release`)*          |
| `GITEA_PASSWORD`         | *(empty — must be set for `make release`)*          |
| `GITEA_PACKAGE_URL`      | `https://gitea.home/api/packages/Silvas/debian`    |
| `GITEA_DEB_DISTRIBUTION` | `stable`                                            |
| `GITEA_DEB_COMPONENT`    | `main`                                              |

```shell
make release GITEA_USERNAME=myuser GITEA_PASSWORD=mytoken
make snapshot DEB_MAINTAINER="You <you@example.com>"
```