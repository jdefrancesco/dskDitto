#!/usr/bin/env python3


import os
from pathlib import Path
import sys

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
    
    # Use pathlib for robust path handling and validation
    target_dir = Path(directory_path)

    # Validate the directory path
    if not target_dir.exists():
        raise FileNotFoundError(f"Error: Directory not found at '{directory_path}'")
    if not target_dir.is_dir():
        raise NotADirectoryError(f"Error: Path exists but is not a directory: '{directory_path}'")

    total_size = 0
    file_count = 0
    
    # Use glob to find all files (excluding directories) recursively
    # glob('**/*') finds all entries, then we filter for files
    for item in target_dir.glob('**/*'):
        if item.is_file():
            # item.stat().st_size gives the size in bytes
            total_size += item.stat().st_size
            file_count += 1

    if file_count == 0:
        # Avoid division by zero
        return 0.0
    
    average_size = total_size / file_count
    return average_size

if __name__ == "__main__":

    if len(sys.argv) > 1:
        # Get the path from the command line argument
        target_directory = sys.argv[1]
    else:
        # Use the current working directory as a fallback
        target_directory = os.getcwd() 
        print(f"No directory specified. Using current directory: {target_directory}\n")

    try:
        average_bytes = calculate_average_file_size(target_directory)

        if average_bytes == 0.0:
            print(f"Directory '{target_directory}' contains no files.")
        else:
            # Helper function to format the size nicely
            def format_size(size_in_bytes):
                # Define units
                units = ['B', 'KB', 'MB', 'GB', 'TB']
                i = 0
                while size_in_bytes >= 1024 and i < len(units) - 1:
                    size_in_bytes /= 1024.0
                    i += 1
                return f"{size_in_bytes:.2f} {units[i]}"
            
            formatted_average = format_size(average_bytes)

            print("-" * 50)
            print(f"Results for directory: {target_directory}")
            print(f"Average File Size (Bytes): {average_bytes:,.2f} B")
            print(f"Average File Size (Formatted): {formatted_average}")
            print("-" * 50)

    except (FileNotFoundError, NotADirectoryError) as e:
        print(f"\nExecution failed: {e}")
    except Exception as e:
        print(f"\nAn unexpected error occurred: {e}")