# cslt-tool

cslt-tool is a convenient wrapper of [arduino-cli](https://github.com/arduino/arduino-cli), it compiles Arduino sketches outputting object files and a json file in a `build/` directory
The json contains information regarding libraries and core to use in order to build the sketch. The result is achieved by parsing the verbose output of `arduino-cli`.

## Requisites
In order to run this tool you have to install first the [arduino-cli](https://github.com/arduino/arduino-cli) and have `arduino-cli` binary in your path, otherwise `cslt-tool` won't work.

## Build it
In order to build it just use `go build`

## Usage
`./cslt-tool compile -b <fqbn> <sketch_path>`

This is an example execution:
``` bash
$ ./cslt-tool compile -b arduino:samd:mkrwan1310 /home/umberto/getdeveui
INFO[0001] arduino-cli version: 0.20.2                  
INFO[0001] running: arduino-cli compile -b arduino:samd:mkrwan1310 /home/umberto/getdeveui -v --format json 
INFO[0002] copied file to /home/umberto/Nextcloud/8tb/Lavoro/cslt-tool/build/getdeveui.ino.cpp.o 
INFO[0002] created new file in: /home/umberto/Nextcloud/8tb/Lavoro/cslt-tool/build/result.json
```
The structure of the `build` forder is the following:
```
build/
├── getdeveui.ino.cpp.o
└── result.json
```
And the content of `build/result.json` is:
```json
{
 "coreInfo": {
  "packager": "arduino",
  "name": "samd",
  "version": "1.8.12"
 },
 "libsInfo": [
  {
   "name": "MKRWAN",
   "version": "1.1.0"
  }
 ]
}
```