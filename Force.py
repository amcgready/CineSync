import os
import sys
import time
import subprocess
from pathlib import Path

def print_ascii_art():
    art = """
A
 _____ _            _____                  
/  __ (_)          /  ___|                 
| /  \/_ _ __   ___\ `--. _   _ _ __   ___ 
| |   | | '_ \ / _ \`--. \ | | | '_ \ / __|
| \__/\ | | | |  __/\__/ / |_| | | | | (__ 
 \____/_|_| |_|\___\____/ \__, |_| |_|\___|
                           __/ |           Tool by amcgready
                          |___/  "May the Force be with you"       
    """
    print(art)

def get_user_choice():
    print("\nSelect an option:")
    print("1) General Scan")
    print("2) TV Scan")
    print("3) Movie Scan")
    print("4) Exit")
    return input("Enter choice (1-4): ").strip()

def process_scan(files_to_scan, scan_type):
    for index, file in enumerate(files_to_scan):
        file = file.strip("'").strip('"')  # Remove surrounding quotes
        file = os.path.abspath(file)
        print(f"[DEBUG] Processing file {index + 1}/{len(files_to_scan)}: {file}")
        
        if not os.path.exists(file):
            print(f"[ERROR] File not found: {file}. Skipping.")
            continue
        
        command = f'python3 MediaHub/main.py "{file}"'
        if scan_type == "General":
            command += " --force"
        elif scan_type in ["TV", "Movie"]:
            command += " --force-show --force"
        
        print(f"[DEBUG] Running Command: {command}")
        try:
            subprocess.run(command, shell=True, check=True)
        except subprocess.CalledProcessError as e:
            print(f"[ERROR] MediaHub command failed for {file}: {e}. Skipping to next file.")
            continue
        
        print(f"[INFO] Ensuring correct query response for: {file}")
        time.sleep(3)  # Allow query response to finalize
        
        print("Refreshing Plex...")
        try:
            subprocess.run('python3 MediaHub/main.py --plex-refresh', shell=True, check=True)
        except subprocess.CalledProcessError as e:
            print(f"[WARNING] Plex refresh failed: {e}. Continuing with next file.")
        
        print(f"[INFO] Completed processing: {file}")
        print("[DEBUG] Moving to next file in batch...")
        time.sleep(2)  # Short delay before moving to the next file
    
    return "continue"

def post_scan_menu():
    while True:
        print("\nAll files processed. What would you like to do?")
        print("1) Continue Scanning")
        print("2) Exit")
        choice = input("Enter choice (1-2): ").strip()
        if choice == '1':
            os.system('clear' if os.name == 'posix' else 'cls')  # Clear console
            return "continue"
        elif choice == '2':
            sys.exit("[INFO] Exiting...")
        else:
            print("[ERROR] Invalid choice, try again.")

def main():
    while True:
        print_ascii_art()
        choice = get_user_choice()
        
        if choice == '4' or choice.lower() == 'exit':
            sys.exit("[INFO] Exiting...")
        
        scan_type = ""
        if choice == '1':
            scan_type = "General"
        elif choice == '2':
            scan_type = "TV"
        elif choice == '3':
            scan_type = "Movie"
        else:
            print("[ERROR] Invalid choice, try again.")
            continue
        
        file_or_folder = input("Drag and drop file(s) or folder here: ").strip()
        print(f"[DEBUG] Raw Input Path: {file_or_folder}")
        
        file_or_folder = file_or_folder.strip("'").strip('"')
        absolute_path = os.path.abspath(file_or_folder)
        
        files_to_scan = []
        if os.path.isdir(absolute_path):
            print(f"[INFO] Folder detected: {absolute_path}. Scanning all files.")
            files_to_scan = sorted([str(f) for f in Path(absolute_path).rglob('*') if f.is_file()])
        elif os.path.isfile(absolute_path):
            files_to_scan.append(absolute_path)
        else:
            print(f"[ERROR] Invalid file or folder: {absolute_path}. Try again.")
            continue
        
        print(f"[DEBUG] Files to be processed: {len(files_to_scan)} files")
        
        scan_result = process_scan(files_to_scan, scan_type)
        if scan_result == "continue":
            continue

if __name__ == "__main__":
    main()
