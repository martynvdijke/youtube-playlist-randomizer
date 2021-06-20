# youtube-playlist-randomizer

> Takes a youtube list shuffles it around and saves it as a new playlist

[![PyPI Version][pypi-image]][pypi-url]
<!-- [![Build Status][build-image]][build-url] -->

## Summary
Since the Chromecast can not shuffle an youtube list, and I sometimes want to just lay back and watch my favourite playlist in random order i came up with this pacakge.
This is a simple python package that takes a youtube playlist as input randomizes it and saves the youtube playlist.
## Installation

```sh
pip install youtube-playlist-randomizer
```
### CLI Documentation

```python
  -h, --help            show this help message and exit
  --version             show programs version number and exit
  -n                    specify the amount of inserts to execute
  -v, --verbose         set loglevel to INFO
  -vv, --very-verbose   set loglevel to DEBUG
  -i INPUT, --input INPUT
                        specify the secret client json file default='client_secret.json'
```
### Config
This scripts needs a client_secret.json file you can get one by going to 
[google docs](https://developers.google.com/youtube/v3/quickstart/python)
and follow step 1 of that tutorial and save the client_secret.json to disk.

## Usage
After installing and getting the client secret file just run and follow the instructions in the folder you saved the json file.
```sh
youtube-playlist-randomizer -i client_secret.json
```


## [Changelog](CHANGELOG.md)

## License

[MIT](https://choosealicense.com/licenses/mit/)

<!-- Badges -->

[pypi-image]: https://img.shields.io/pypi/v/youtube-playlist-randomizer
[pypi-url]: https://pypi.org/project/youtube-playlist-randomizer
[build-image]: https://github.com/martynvdijke/gr-lora_sdr-profiler/actions/workflows/build.yml/badge.svg
[build-url]: https://github.com/martynvdijke/gr-lora_sdr-profiler/actions/workflows/build.yml