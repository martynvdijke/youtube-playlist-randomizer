from youtube_playlist_randomizer import auth, arg_parser
from youtube_playlist_randomizer.randomizer import Randomizer


def main():
    args = arg_parser.parse_args()
    file = args.input
    youtube = auth.auth(file)
    randomizer = Randomizer(youtube, args)
    randomizer.run()
