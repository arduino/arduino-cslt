# cslt-tool

`cslt-tool` is a convenient wrapper of [arduino-cli](https://github.com/arduino/arduino-cli), it compiles Arduino sketches outputting a precompiled library in the current working directory.
It generates a json file in the `extras/` folder that contains information regarding libraries and core to use in order to build the sketch. The result is achieved by parsing the verbose output of `arduino-cli` and by using [GNU ar](https://sourceware.org/binutils/docs/binutils/ar.html) to generate an archive of the object files.

## Prequisites
In order to run this tool you have to install first the [Arduino CLI](https://github.com/arduino/arduino-cli) and have `arduino-cli` binary in your `$PATH`, otherwise `cslt-tool` won't work.
Please use a version of the Arduino CLI that has [this](https://github.com/arduino/arduino-cli/pull/1608) change (version > 0.20.2).

Another requirement is [`gcc-ar`](https://sourceware.org/binutils/docs/binutils/ar.html) (installable with `apt-get install gcc`) in your `$PATH`.

## Build it
In order to build `cslt-tool` just use `go build`

## Usage
`./cslt-tool compile -b <fqbn> <sketch_path>`

For example, running `./cslt-tool compile -b arduino:samd:mkrwifi1010 sketch/sketch.ino` should produce a library with the following structure, in the current working directory:
```
libsketch/
├── examples
│   └── sketch
│       └── sketch.ino  <-- the actual sketch we are going to compile with the arduino-cli later
├── extras
│   └── result.json
├── library.properties
└── src
    ├── cortex-m0plus
    │   └── libsketch.a
    └── libsketch.h
```

This is an example execution:
```
$ ./cslt-tool compile -b arduino:samd:mkrwifi1010 sketch/sketch.ino
INFO[0000] arduino-cli version: git-snapshot            
INFO[0000] GNU ar (GNU Binutils) 2.37                   
INFO[0000] the ino file path is sketch/sketch.ino 
INFO[0000] created sketch/main.cpp 
INFO[0000] replaced setup() and loop() functions in sketch/sketch.ino 
INFO[0000] running: arduino-cli compile -b arduino:samd:mkrwifi1010 sketch/sketch.ino -v --format json 
INFO[0000] running: arduino-cli compile -b arduino:samd:mkrwifi1010 sketch/sketch.ino --show-properties 
INFO[0001] removed sketch/main.cpp 
INFO[0001] created sketch/sketch.ino 
INFO[0001] restored sketch/sketch.ino 
INFO[0001] created libsketch/library.properties 
INFO[0001] created libsketch/src/libsketch.h 
INFO[0001] created libsketch/examples/sketch/sketch.ino 
INFO[0001] running: gcc-ar rcs libsketch/src/cortex-m0plus/libsketch.a /tmp/arduino-sketch-E4D76B1781E9EB73A7B3491CAC68F374/sketch/sketch.ino.cpp.o 
INFO[0001] created libsketch/src/cortex-m0plus/libsketch.a 
INFO[0001] created libsketch/extras/result.json
```

And the content of `libsketch/extras/result.json` is:
```json
{
 "coreInfo": {
  "id": "arduino:samd",
  "version": "1.8.12"
 },
 "libsInfo": [
  {
   "name": "WiFiNINA",
   "version": "1.8.13",
   "provides_includes": [
    "WiFiNINA.h"
   ]
  },
  {
   "name": "SPI",
   "version": "1.0",
   "provides_includes": [
    "SPI.h"
   ]
  }
 ]
}
```

## How to compile the precompiled sketch
In order to compile the sketch you have first to install manually the libraries and the core listed in the `<libsketch>/extras/result.json` file.

You can install a library with [`arduino-cli lib install LIBRARY[@VERSION_NUMBER]`](https://arduino.github.io/arduino-cli/latest/commands/arduino-cli_lib_install/).

You can install a core with [`arduino-cli core install PACKAGER:ARCH[@VERSION]`](https://arduino.github.io/arduino-cli/latest/commands/arduino-cli_core_install/).

After completing that operation you can compile it with:

`arduino-cli compile -b <fqbn> <libsketch>/examples/sketch/sketch.ino --library <libsketch>`.

It's important to use the `--library` flag to include the precompiled library generated with cslt-tool otherwise the Arduino CLI won't find it.

For example a legit execution looks like this:
```
$ arduino-cli compile -b arduino:samd:mkrwifi1010 libsketch/examples/sketch/sketch.ino --library libsketch/

Library libsketch has been declared precompiled:
Using precompiled library in libsketch/src/cortex-m0plus
Sketch uses 14636 bytes (5%) of program storage space. Maximum is 262144 bytes.
Global variables use 3224 bytes (9%) of dynamic memory, leaving 29544 bytes for local variables. Maximum is 32768 bytes.
```