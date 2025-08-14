# youtube-playlist-randomizer

> Takes a youtube list shuffles it around and saves it as a new playlist

[![PyPI Version][pypi-image]][pypi-url]
<!-- [![Build Status][build-image]][build-url] -->

## Summary

Since the Chromecast can not shuffle an youtube list, and I sometimes want to just lay back and watch my favourite playlist in random order i came up with this package.
This is a simple python package that takes a youtube playlist as input randomizes it and saves the youtube playlist.

## Installation

```sh
pip install youtube-playlist-randomizer
```

### CLI Documentation

```python
Playlist randomizer

options:
  -h, --help            show this help message and exit
  --version             show program's version number and exit
  -v, --verbose         set loglevel to INFO
  -vv, --very-verbose   set loglevel to DEBUG
  -n CHUNKS, --number_of CHUNKS
                        Specify the number of update request to do per 24 hours [default=190]
  -i INPUT, --input INPUT
                        Specify the secret client json file [default='client_secret.json']
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