import os
import re
import requests
from MediaHub.utils.logging_utils import log_message
from MediaHub.utils.file_utils import fetch_json, extract_resolution, extract_resolution_from_folder, get_anime_patterns
from MediaHub.api.tmdb_api import search_tv_show
from MediaHub.config.config import *
from MediaHub.utils.mediainfo import *
from MediaHub.api.tmdb_api_helpers import get_episode_name

def is_anime_file(filename):
    """
    Detect if the file is likely an anime file based on naming patterns
    """
    anime_pattern = get_anime_patterns()
    return bool(anime_pattern.search(filename))

def extract_anime_episode_info(filename):
    """
    Extract anime-specific episode information from the provided filename.
    Returns a dictionary with show_name, season_number, episode_number, and episode_title.
    """

    clean_filename = filename
    clean_filename = re.sub(r'^\[(.*?)\]', '', clean_filename)
    clean_filename = re.sub(r'\[[A-F0-9]{8}\](?:\.[^.]+)?$', '', clean_filename)
    clean_filename = re.sub(r'\[.*?\]', '', clean_filename)
    clean_filename = re.sub(r'\(.*?\)', '', clean_filename)
    clean_filename = os.path.splitext(clean_filename)[0]
    clean_filename = re.sub(r'\s+', ' ', clean_filename).strip()

    # Check for special anime pattern S##S## (Season + Special)
    special_pattern = r'^(.+?)\s*-\s*S(\d+)S(\d+)(?:\s|$)'
    match = re.match(special_pattern, clean_filename, re.IGNORECASE)
    if match:
        show_name = match.group(1).strip()
        season_number = str(int(match.group(2))).zfill(2)
        special_number = str(int(match.group(3))).zfill(2)

        show_name = re.sub(r'[._-]', ' ', show_name).strip()
        log_message(f"Identfied Special Episode for show: {show_name}, Season: {season_number}, Special: {special_number}.", level="DEBUG")
        return {
            'show_name': show_name,
            'season_number': season_number,
            'episode_number': special_number,
            'episode_title': None,
            'is_extra': True,
        }

    season_detection_patterns = [
        r'^(.+?)\s*S(\d+)\s*-\s*(\d+)$',
        r'^(.+?)\s*Season\s*(\d+)\s*Episode\s*(\d+)',
        r'^(.+?)\s*Season\s*(\d+)[-_\s]*Episode\s*(\d+)',
        r'^(.+?)\s*Season\s*(\d+)[-_\s]*(?:-\s*)?(\d+)'
    ]

    for pattern in season_detection_patterns:
        match = re.match(pattern, clean_filename, re.IGNORECASE)
        if match:
            show_name = match.group(1).strip()
            season_number = str(int(match.group(2))).zfill(2)
            episode_number = str(int(match.group(3))).zfill(2)

            show_name = re.sub(r'[._-]', ' ', show_name).strip()

            return {
                'show_name': show_name,
                'season_number': season_number,
                'episode_number': episode_number,
                'episode_title': None
            }

    ordinal_season_patterns = [
        r'^(.+?)\s+(\d+)(?:st|nd|rd|th)\s+Season[-_\s]*(?:-\s*)?(\d+)(?:\s|$)',
        r'^(.+?)\s+(\d+)(?:st|nd|rd|th)\s+Season.*?[-_](\d+)(?:\s|$)',
        r'^(.+?)\s*S(\d+)(?:\s|$|E)'
    ]

    for pattern in ordinal_season_patterns:
        match = re.match(pattern, clean_filename, re.IGNORECASE)
        if match:
            show_name = match.group(1).strip()
            season_number = str(int(match.group(2))).zfill(2)

            # For the third pattern, we're only capturing season number, not episode
            if len(match.groups()) == 2:
                return {
                    'show_name': show_name,
                    'season_number': season_number,
                    'episode_number': None,
                    'episode_title': None
                }
            else:
                episode_number = str(int(match.group(3))).zfill(2)
                if len(episode_number) <= 3:
                    return {
                        'show_name': show_name,
                        'season_number': season_number,
                        'episode_number': episode_number,
                        'episode_title': None
                    }

    # Pattern for versioned episode numbers (like 17v2)
    versioned_episode_patterns = [
        r'^(.+?)[-_\s]+(\d+)v\d+[-_\s]*(?:\s|$)',
    ]

    for pattern in versioned_episode_patterns:
        match = re.match(pattern, clean_filename, re.IGNORECASE)
        if match:
            show_name = match.group(1).strip()
            episode_number = str(int(match.group(2))).zfill(2)

            show_name = re.sub(r'[._-]', ' ', show_name).strip()

            return {
                'show_name': show_name,
                'season_number': None,
                'episode_number': episode_number,
                'episode_title': None
            }

    # Pattern for simple show name + episode number format
    simple_episode_patterns = [
        r'^(.+?)\s+(\d{1,3})(?:\s|$)',
        r'^(.+?)\s*-\s*(\d{1,3})(?:\s|$)',
        r'^(.+?)\s*EP?\.?\s*(\d{1,3})(?:\s|$)',
        r'^(.+?)\s+(\d{4})(?:\s|$)',
    ]

    for pattern in simple_episode_patterns:
        match = re.match(pattern, clean_filename, re.IGNORECASE)
        if match:
            show_name = match.group(1).strip()
            episode_number = str(int(match.group(2))).zfill(2)

            show_name = re.sub(r'[._-]', ' ', show_name).strip()

            return {
                'show_name': show_name,
                'season_number': None,
                'episode_number': episode_number,
                'episode_title': None
            }

    anime_patterns = [
        r'^(.+?)\s*S(\d+)\s*-\s*.*?-\s*(\d+)$',
        r'^(.+?)\s*-\s*(\d+)\s*(?:-\s*(.+))?$',
        r'^(.+?)\s*S(\d{2})E(\d+)\s*(?:-\s*(.+))?$',
        r'^(.+?)\s*(\d+)x(\d+)\s*(?:-\s*(.+))?$',
        r'^(.+?)\s*(?:[Ee]p\.?\s*(\d+)|[Ee]pisode\s*(\d+))\s*(?:-\s*(.+))?$',
        r'^(.+?)\s*\[(\d+)\]\s*(?:-\s*(.+))?$',
    ]

    for pattern_index, pattern in enumerate(anime_patterns, 1):
        match = re.match(pattern, clean_filename, re.IGNORECASE)
        if match:
            if pattern_index == 1:
                show_name = match.group(1).strip()
                season_number = match.group(2).zfill(2)
                episode_number = match.group(3).zfill(2)
                episode_title = None
            elif pattern_index == 2:
                show_name = match.group(1).strip()
                season_number = None
                episode_number = match.group(2).zfill(2)
                episode_title = match.group(3)
            elif pattern_index == 3:
                show_name = match.group(1).strip()
                season_number = match.group(2).zfill(2)
                episode_number = match.group(3).zfill(2)
                episode_title = match.group(4)
            else:
                show_name = match.group(1).strip()
                season_number = None
                episode_number = match.group(2).zfill(2)
                episode_title = match.group(3) if len(match.groups()) > 2 else None

            show_name = re.sub(r'[._-]', ' ', show_name).strip()

            return {
                'show_name': show_name,
                'episode_number': episode_number,
                'season_number': season_number,
                'episode_title': episode_title
            }

    return None

def process_anime_show(src_file, root, file, dest_dir, actual_dir, tmdb_folder_id_enabled, rename_enabled, tmdb_id, tvdb_id, imdb_id, auto_select, season_number, episode_number):
    anime_info = extract_anime_episode_info(file)
    if not anime_info:
        return None

    # Prepare variables
    show_name = anime_info['show_name']
    season_number = season_number or anime_info['season_number']
    episode_number = episode_number or anime_info['episode_number']
    episode_title = anime_info['episode_title']
    is_extra = anime_info.get('is_extra', False)

    # Extract resolution from filename and parent folder
    file_resolution = extract_resolution(file)
    folder_resolution = extract_resolution_from_folder(os.path.basename(root))
    resolution = file_resolution or folder_resolution
    media_info = {}
    resolution = resolution.lower() if resolution is not None else None

    # Check for media info
    root_folder_name = os.path.basename(os.path.dirname(root))
    if root_folder_name:
        root_media_info = extract_media_info(root_folder_name, keywords)
        media_info.update(root_media_info)

    if actual_dir:
        actual_dir_media_info = extract_media_info(actual_dir, keywords)
        media_info.update(actual_dir_media_info)

    file_media_info = extract_media_info(file, keywords)
    media_info.update(file_media_info)

    # Clean up show name
    show_name = re.sub(r'[._]', ' ', show_name).strip()

    # Fetch proper show name and ID from TMDb
    year = None
    proper_show_name = show_name
    original_show_name = show_name
    show_id = None
    is_anime_genre = False

    search_result = search_tv_show(show_name, auto_select=auto_select, season_number=season_number, episode_number=episode_number, tmdb_id=tmdb_id, imdb_id=imdb_id, tvdb_id=tvdb_id, is_extra=is_extra, file=file)
    # Check if result is None (API connection issues)
    if search_result is None:
        log_message(f"API returned None for show: {show_name} ({year}). Skipping Anime show processing.", level="WARNING")
        return None
    elif isinstance(search_result, tuple):
        proper_show_name, original_show_name, is_anime_genre, season_number, episode_number, tmdb_id = search_result
    else:
        proper_show_name = original_show_name = search_result

    tmdb_id_match = re.search(r'\{tmdb-(\d+)\}$', proper_show_name)
    if tmdb_id_match:
        show_id = tmdb_id_match.group(1)

    if is_tmdb_folder_id_enabled():
        show_name = proper_show_name
    elif is_imdb_folder_id_enabled():
        show_name = re.sub(r' \{tmdb-.*?\}$', '', proper_show_name)
    else:
        show_name = re.sub(r' \{(?:tmdb|imdb)-.*?\}$', '', proper_show_name)

    new_name = file
    episode_name = None
    mapped_season = season_number
    mapped_episode = episode_number

    # Parse the original filename to get the correct episode number
    original_episode_match = re.search(r'S(\d{2})E(\d{2})', file)
    if original_episode_match:
        season_number = original_episode_match.group(1)
        actual_episode = original_episode_match.group(2)
    else:
        original_episode_match = re.search(r'(\d+)x(\d+)', file)
        if original_episode_match:
            season_number = str(int(original_episode_match.group(1))).zfill(2)
            actual_episode = original_episode_match.group(2)
        else:
            actual_episode = episode_number

    if rename_enabled and show_id:
        try:
            try:
                # Get the episode name and the mapped season/episode numbers
                episode_result = get_episode_name(show_id, int(season_number), int(actual_episode))

                if isinstance(episode_result, tuple) and len(episode_result) == 3:
                    episode_name, mapped_season, mapped_episode = episode_result
                    # Update season_number with the mapped season number
                    if mapped_season is not None:
                        season_number = str(mapped_season).zfill(2)
                    # Update actual_episode with the mapped episode number
                    if mapped_episode is not None:
                        actual_episode = str(mapped_episode).zfill(2)
                else:
                    episode_name = episode_result

                if episode_name and episode_name != episode_title:
                    new_name += f" - {episode_name}"
                elif episode_title:
                    new_name += f" - {episode_title}"
            except Exception as e:
                log_message(f"Failed to fetch episode name: {e}", level="WARNING")

            episode_name = episode_name or episode_title or ""

            new_name = f"{original_show_name}"
            if episode_name:
                new_name += f" - {episode_name}"
            if resolution:
                new_name += f" [{resolution}]"

            # Add media info tags with separate brackets
            media_tags = []

            if media_info.get('VideoCodec'):
                codec = media_info['VideoCodec']
                if '10bit' in actual_dir or '10bit' in file:
                    media_tags.append(f"[{codec} 10bit]")
                else:
                    media_tags.append(f"[{codec}]")
            if media_info.get('AudioCodec'):
                audio_tag = media_info['AudioCodec']
                if media_info.get('AudioChannels'):
                    audio_tag += f" {media_info['AudioChannels']}"
                if media_info.get('AudioAtmos'):
                    audio_tag += f" {media_info['AudioAtmos']}"
                media_tags.append(f"[{audio_tag}]")
            if media_info.get('DynamicRange'):
                media_tags.append(f"[{media_info['DynamicRange']}]")
            if media_info.get('Languages'):
                if 'ENG' in media_info['Languages'] and len(media_info['Languages']) > 1:
                    media_tags.append("[Dual Audio]")

            if media_tags:
                new_name += f" {''.join(media_tags)}"

            new_name += os.path.splitext(file)[1]

        except Exception as e:
            log_message(f"Error processing anime filename: {e}", level="ERROR")
            new_name = file

    # Return necessary information
    return {
        'show_name': show_name,
        'season_number': season_number,
        'new_name': new_name,
        'year': year,
        'is_anime': True,
        'show_id': show_id,
        'episode_title': episode_name or episode_title,
        'episode_number': actual_episode,
        'resolution': resolution,
        'media_info': media_info,
        'is_anime_genre': is_anime_genre,
        'is_extra': is_extra,
    }
