

from youtube_playlist_randomizer import __version__
import argparse
import logging
import pathlib
import sys


def parse_args(args):
    """
    Args:
        args: cli arguments given to script
    Returns:
        list of supported arguments
    """
    parser = argparse.ArgumentParser(description="Playlist randomizer")
    parser.add_argument(
        "--version",
        action="version",
        version=f"randomizer version: {__version__}",
    )

    # set logging level
    parser.add_argument(
        "-v",
        "--verbose",
        dest="loglevel",
        help="set loglevel to INFO",
        action="store_const",
        const=logging.INFO,
    )
    parser.add_argument(
        "-vv",
        "--very-verbose",
        dest="loglevel",
        help="set loglevel to DEBUG",
        action="store_const",
        const=logging.DEBUG,
    )
    
    parser.add_argument(
        "-n",
        "--update_request",
        default=190,
        help="Specify the number of update request to do per 24 hours [default=%(default)r]",
    )

    parser.add_argument(
        "-i",
        "--input",
        default="client_secret.json",
        type=pathlib.Path,
        help="Specify the secret client json file [default=%(default)r]",
        required=True,
    )
    return parser.parse_args(args)


def setup_logging(loglevel):
    """Setup basic logging
    Args:
      loglevel (int): minimum loglevel for emitting messages
    """
    logformat = "[%(asctime)s] %(levelname)s:%(name)s:%(message)s"
    logging.basicConfig(
        level=loglevel,
        stream=sys.stdout,
        format=logformat,
        datefmt="%Y-%m-%d %H:%M:%S",
    )
    # setup colred logs
    # coloredlogs.install(level=loglevel, logger=_logger)

