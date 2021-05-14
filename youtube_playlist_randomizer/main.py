"""
Main module for
"""
import argparse
import logging
import sys
import pathlib
import coloredlogs
from . import __version__
from . import auth
from . import playlist

__author__ = "Martyn van Dijke"
__copyright__ = "Martyn van Dijke"
__license__ = "MIT"
_logger = logging.getLogger(__name__)


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
    coloredlogs.install(level=loglevel, logger=_logger)


def main(args):
    """
    Main function that does all the dispatching of the subfunctions
    Args:
        args: sys arguments
    Returns:
        none
    """
    args = parse_args(args)
    setup_logging(args.loglevel)
    youtube = auth.auth(args)
    playlist.PlayListRandomizer(youtube)


if __name__ == "__main__":
    # ^  This is a guard statement that will prevent the following code from
    #    being executed in the case someone imports this file instead of
    #    executing it as a script.
    #    https://docs.python.org/3/library/__main__.html
    main(sys.argv[1:])
