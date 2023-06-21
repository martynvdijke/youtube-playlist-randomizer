
from youtube_playlist_randomizer import auth, arg_parser
from youtube_playlist_randomizer.randomizer import Randomizer
import types

def main():
    args = types.SimpleNamespace()
    args.input= "client_secret.json"
    youtube = auth.auth(args)
    tes = Randomizer(youtube, args)
