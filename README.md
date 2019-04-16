# unicast-mpv
> A simple module that exposes [MPV](http://mpv.io/) to be controlled through a [RPC WebSockets](https://github.com/elpheria/rpc-websockets) API, that can be used with the [unicast](https://github.com/pedromsilvapt/unicast) server

## Installation
This module can be used as a terminal application by simply installing it globally:

```shell
npm install -g unicast-mpv
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