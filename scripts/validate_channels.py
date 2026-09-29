#!/usr/bin/env python3
"""Validate the remote channel JSON contract before deployment."""

import json
import sys
from pathlib import Path
from urllib.parse import urlsplit
from urllib.request import urlopen

MAX_BYTES = 1024 * 1024


def valid_url(value):
    if not isinstance(value, str):
        return False
    parts = urlsplit(value.strip())
    return parts.scheme in ("http", "https") and bool(parts.hostname)


def load(source):
    if source.startswith(("https://", "http://")):
        with urlopen(source, timeout=10) as response:
            data = response.read(MAX_BYTES + 1)
    else:
        data = Path(source).read_bytes()
    if len(data) > MAX_BYTES:
        raise ValueError("configuration exceeds 1 MiB")
    return json.loads(data)


def validate(config):
    if not isinstance(config, dict) or config.get("version") != 1:
        raise ValueError("version must be 1")
    channels = config.get("channels")
    if not isinstance(channels, list) or not channels:
        raise ValueError("channels must be a nonempty list")
    seen = set()
    for index, channel in enumerate(channels, 1):
        if not isinstance(channel, dict):
            raise ValueError(f"channel {index} must be an object")
        name = channel.get("name")
        group = channel.get("group", "其他")
        if not isinstance(name, str) or not name.strip():
            raise ValueError(f"channel {index} has no name")
        if not isinstance(group, str) or not group.strip():
            raise ValueError(f"channel {index} has no group")
        identity = (group.strip(), name.strip())
        if identity in seen:
            raise ValueError(f"duplicate channel: {identity}")
        seen.add(identity)
        urls = channel.get("urls")
        if not isinstance(urls, list) or not urls or not all(map(valid_url, urls)):
            raise ValueError(f"channel {index} has an invalid playback URL")
        logo = channel.get("logo")
        if not valid_url(logo):
            raise ValueError(f"channel {index} has an invalid logo URL")
    return len(channels)


if __name__ == "__main__":
    try:
        count = validate(load(sys.argv[1]))
    except (IndexError, OSError, ValueError, json.JSONDecodeError) as error:
        sys.exit(f"Invalid channel configuration: {error}")
    print(f"Valid channel configuration: {count} channels")
