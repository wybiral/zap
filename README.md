# zap
MicroPython CLI tool.

## Commands
```
   cat       Read file
   cd        Change directory
   download  Copy all files from device to local directory
   get       Copy a file from the device
   help      Shows all commands or help for one command
   ls        List files
   mkdir     Make directory
   put       Copy a file to the device
   pwd       Print working directory
   reboot    Perform a soft reboot
   repl      Open the MicroPython REPL
   rm        Delete file
   rmdir     Remove directory
   upload    Copy all files from local directory to device
   version   Print zap version
```
## Examples

The easiest way to use zap is to set the environment variable `PYBOARD_DEVICE` to whatever serial device your board is connected to. On Windows that could be something like `COM1`, `COM2`, etc. On Linux it may be something like `/dev/ttyACM0`. If you don't want to use an environment variable you can supply the `--device` or `-d` flag.

If there's code running in the background it can interfere with any of these actions so it's usually best to run `zap reboot` which will perform a soft reboot and stop any existing code.

Enter the MicroPython REPL:
```
zap repl
```

Download all files from the current directory of the MicroPython device to the local directory:
```
zap download
```

Upload all from from the local directory to the current directory of the MicroPython device:
```
zap upload
```

Change current working directory to `lib`:
```
zap cd lib
```
