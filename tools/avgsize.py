#!/usr/bin/env python3

import sys
import stat
import os


def format_size(size_in_bytes: float) -> str:
    """bytes -> human-readable units"""
    units = ["B", "KB", "MB", "GB", "TB"]
    i = 0
    while size_in_bytes >= 1024 and i < len(units) - 1:
        size_in_bytes /= 1024.0
        i += 1
    return f"{size_in_bytes:.2f} {units[i]}"


def calculate_average_file_size(directory_path: str) -> float:
    """
    Calculates the average size of files within the specified directory.

    Args:
        directory_path: The path to the directory to scan.

    Returns:
        The average file size in bytes, or 0.0 if no files are found.

    Raises:
        FileNotFoundError: If the directory path does not exist.
        NotADirectoryError: If the path exists but is not a directory.
    """
    if not os.path.exists(directory_path):
        raise FileNotFoundError(f"Error: Directory not found at: '{directory_path}'")
    if not os.path.isdir(directory_path):
        raise NotADirectoryError(
            f"Error: Path exists but is not a directory: '{directory_path}'"
        )
    if not os.access(directory_path, os.R_OK):
        raise PermissionError(f"Error: No permissions to read: '{directory_path}'")
    total_size = 0
    file_count = 0
    for root, _, files in os.walk(directory_path):
        for name in files:
            file_path = os.path.join(root, name)
            try:
                file_stat = os.stat(file_path)
                # exclude sockets, devices, etc.
                if stat.S_ISREG(file_stat.st_mode):
                    total_size += file_stat.st_size
                    file_count += 1
            except (PermissionError, OSError):
                continue
    if file_count == 0:
        return 0.0
    return total_size / file_count


if __name__ == "__main__":
    if len(sys.argv) > 1:
        target_directory = sys.argv[1]
    else:
        target_directory = os.getcwd()
        print(f"No directory specified. Using current directory: {target_directory}\n")
    try:
        average_bytes = calculate_average_file_size(target_directory)
        if average_bytes == 0.0:
            print(f"Directory '{target_directory}' contains no files.")
        else:
            formatted_average = format_size(average_bytes)
            print("-" * 50)
            print(f"Results for directory: {target_directory}")
            print(f"Average File Size (Bytes): {average_bytes:,.2f} B")
            print(f"Average File Size (Formatted): {formatted_average}")
            print("-" * 50)
    except (FileNotFoundError, NotADirectoryError, PermissionError) as e:
        print(f"{e}")
    except Exception as e:
        print(f"An unexpected error occurred: {e}")
