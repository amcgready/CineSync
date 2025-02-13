import os
import sys
import subprocess
import random
import getpass
import logging
import subprocess
import sys
import pkg_resources
import platform
import sqlite3
import json
import time
import glob
import threading
import atexit
from datetime import datetime
from discord_webhook import DiscordWebhook, DiscordEmbed
from dotenv import load_dotenv


# Append the parent directory to the system path
sys.path.append(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

# Local imports from MediaHub
from MediaHub.utils.logging_utils import log_message
from MediaHub.processors.db_utils import get_database_stats, vacuum_database, verify_database_integrity, export_database, import_database, search_database, optimize_database, reset_database

# Script Metadata
SCRIPT_VERSION = "2.2"
SCRIPT_DATE = "2025-01-13"

# Load environment variables
load_dotenv()
DISCORD_WEBHOOK_URL = os.getenv("DISCORD_WEBHOOK_URL")
MONITORING_INTERVAL = int(os.getenv("MONITORING_INTERVAL", 300))  # Default to 5 minutes
monitoring_enabled = False  # Flag to enable monitoring after a scan starts


# Define log directory
LOGS_FOLDER = "logs"

# Define variables
SCRIPTS_FOLDER = "MediaHub"
BROKEN_LINK_FOLDER = "BrokenLinkVault"
MONITOR_SCRIPT = os.path.join(SCRIPTS_FOLDER, "utils/service_manager.py")
ENV_FILE = ".env"
LIBRARY_SCRIPT = os.path.join(SCRIPTS_FOLDER, "main.py")
BROKEN_LINKS_SCRIPT = os.path.join(SCRIPTS_FOLDER, "utils/broken_links.py")

# Setup logging
logging.basicConfig(filename='script.log', level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

# Determine the Python command based on the OS
python_command = 'python' if platform.system() == 'Windows' else 'python3'

# Function to print text with color
def print_color(text, color):
    colors = {
        "red": "\033[91m",
        "green": "\033[92m",
        "yellow": "\033[93m",
        "blue": "\033[94m",
        "end": "\033[0m",
    }
    print(f"{colors.get(color, '')}{text}{colors.get('end', '')}")

# Function to check and install required Python packages
def check_python_and_dependencies(required_version="3.6"):
    try:
        version_output = subprocess.run([sys.executable, '--version'], check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        version_str = version_output.stdout.decode().strip().split()[1]
        major, minor, micro = map(int, version_str.split('.'))
        current_version = f"{major}.{minor}"
        print(f"Current Python version: {current_version}")

        if (major, minor) < tuple(map(int, required_version.split('.'))):
            print_color(f"Python {required_version} or higher is required. Current version is {current_version}.", "red")
            input("Press Enter to exit the script...")
            sys.exit(1)
    except FileNotFoundError:
        print_color("Python is not installed. Please install Python 3.6 or higher.", "red")
        input("Press Enter to exit the script...")
        sys.exit(1)
    except subprocess.CalledProcessError as e:
        print_color(f"Error checking Python version: {e}", "red")
        input("Press Enter to exit the script...")
        sys.exit(1)

    # Check and install dependencies
    required_packages = []
    if not os.path.exists('requirements.txt'):
        print_color("Error: requirements.txt file not found.", "red")
        input("Press Enter to exit the script...")
        sys.exit(1)

    with open('requirements.txt', 'r') as file:
        for line in file:
            package = line.strip()
            if package:
                required_packages.append(package)

    installed_packages = [pkg.key for pkg in pkg_resources.working_set]

    missing_packages = [pkg for pkg in required_packages if pkg.lower() not in installed_packages]

    if missing_packages:
        print_color("Missing packages detected. Installing...", "yellow")
        try:
            subprocess.check_call([sys.executable, '-m', 'pip', 'install', *missing_packages])
            print_color("All missing packages installed successfully.", "green")
        except subprocess.CalledProcessError as e:
            print_color(f"Error installing packages: {e}", "red")
            input("Press Enter to exit the script...")
            sys.exit(1)
    else:
        print_color("All required packages are already installed.", "green")

check_python_and_dependencies()

# Function to clear the screen
def clear_screen():
    os.system('cls' if os.name == 'nt' else 'clear')

# Function to print the banner
def print_banner():
    print("""
    a88888b. oo                   .d88888b
   d8'   `88                      88.    "'
   88        dP 88d888b. .d8888b. `Y88888b. dP    dP 88d888b. .d8888b.
   88        88 88'  `88 88ooood8       `8b 88    88 88'  `88 88'  `"`
   Y8.   .88 88 88    88 88.  ... d8'   .8P 88.  .88 88    88 88.  ...
    Y88888P' dP dP    dP `88888P'  Y88888P  `8888P88 dP    dP `88888P'
                                                 .88
                                             d8888P
    """)
    print(f"\nVersion {SCRIPT_VERSION} - Last updated on {SCRIPT_DATE}\n")

# Function to print a random welcome message
def print_random_welcome(username):
    welcome_messages = [
        f"Welcome back, {username}! Ready to manage your library?",
        f"Hey there, {username}! Your library adventure begins now!",
        f"Hello, {username}! Let's dive into your library management system!",
        f"Greetings, {username}! Get ready to organize your library!",
    ]
    random_messages = [
        "Did you know? Organizing your library can reduce stress!",
        "Tip of the day: Keep your library updated to avoid confusion.",
        "Fact: Well-organized libraries can improve productivity!",
        "Remember: Regular scans help keep your library tidy.",
    ]
    print(f"\n{random.choice(welcome_messages)}")
    print(f"Random Tip: {random.choice(random_messages)}\n")

# Function to greet the user
def greet_user():
    username = getpass.getuser()
    print_random_welcome(username)

# Function to edit the .env file
def edit_env_file():
    try:
        if os.path.exists(ENV_FILE):
            subprocess.run([python_command, '-m', 'nano', ENV_FILE], check=True)
            print("\n.env file editing completed.")
        else:
            print("The .env file does not exist. Creating a new one.")
            with open(ENV_FILE, 'w') as f:
                pass
            subprocess.run([python_command, '-m', 'nano', ENV_FILE], check=True)
            print("\n.env file created and edited.")
    except subprocess.CalledProcessError as e:
        logging.error(f"Error editing .env file: {e}")
        print_color("Error editing .env file. Check the log for details.", "red")
    input("Press Enter to return to the main menu...")

# Function for Real-Time Monitoring
def real_time_monitoring():
    if os.uname().sysname != "Linux" and os.geteuid() != 0:
        print_color("Error: This function requires root privileges. Please run the script with sudo.", "red")
        input("Press Enter to return to the main menu...")
        return

    while True:
        clear_screen()
        print_banner()
        print("\nReal-Time Monitoring Options:")
        print("1) Enable Real-Time Monitoring Service")
        print("2) Disable Real-Time Monitoring Service")
        print("3) Exit to Main Menu")
        choice = input("Select an option: ")

        try:
            if choice == '1':
                subprocess.run([python_command, MONITOR_SCRIPT, 'enable'], check=True)
                input("Press Enter to continue...")
            elif choice == '2':
                subprocess.run([python_command, MONITOR_SCRIPT, 'disable'], check=True)
                input("Press Enter to continue...")
            elif choice == '3':
                break
            else:
                print_color("Invalid option. Please select again.", "red")
                input("Press Enter to continue...")
        except subprocess.CalledProcessError as e:
            logging.error(f"Error executing real-time monitoring command: {e}")
            print_color("Error executing command. Check the log for details.", "red")

    if os.uname().sysname != "Linux":
        print_color("Warning: Real-Time Monitoring is only available on Linux OS.", "yellow")
        input("Press Enter to return to the main menu...")

# Function to send Discord notifications
def send_discord_notification(message, color, file_path=None):
    if not DISCORD_WEBHOOK_URL:
        logging.error("Discord webhook URL is not set.")
        return
    
    webhook = DiscordWebhook(url=DISCORD_WEBHOOK_URL)
    embed = DiscordEmbed(description=message, color=color)  # Use only description, no title
    embed.set_timestamp()
    webhook.add_embed(embed)

    # Prevent NoneType errors in file handling
    if file_path and os.path.exists(file_path):
        try:
            with open(file_path, "rb") as file:
                webhook.add_file(file=file.read(), filename=os.path.basename(file_path))
        except Exception as e:
            logging.error(f"Error attaching file {file_path}: {e}")

    webhook.execute()

# Function to get the latest log file
def get_latest_log_file():
    log_files = sorted(glob.glob(os.path.join(LOGS_FOLDER, "*.log")), key=os.path.getmtime, reverse=True)
    return log_files[0] if log_files else ""  # Ensures it never returns None

# Notify when a scan starts
def notify_scan_started(scan_type):
    global monitoring_enabled
    message = f"📘 **Scan Started:** {scan_type}"
    send_discord_notification(message, color=3447003)
    monitoring_enabled = True  # Enable monitoring after the first scan starts

# Notify when the script is terminated
def notify_script_terminated():
    log_file = get_latest_log_file()
    message = "🚨 **Script Terminated:** The script has been stopped or closed unexpectedly."
    send_discord_notification(message, color=15158332, file_path=log_file)

# Notify monitoring status
def notify_monitoring():
    if not monitoring_enabled:
        return  # Do not send monitoring notifications until a scan has started and interval is met
    
    log_file = get_latest_log_file()
    warning_count, error_count, critical_count = count_log_issues(log_file) if log_file else (0, 0, 0)
    
    message = (f"🟢 **Actively Monitoring**\n"
               f"Warnings: **{warning_count}** 🟡\n"
               f"Errors: **{error_count}** 🟠\n"
               f"Critical: **{critical_count}** 🔴")
    send_discord_notification(message, color=3066993, file_path=log_file)

# Count issues in logs
def count_log_issues(log_file):
    if not log_file or not os.path.exists(log_file):  # Ensure file exists
        return 0, 0, 0

    warning_count = error_count = critical_count = 0
    try:
        with open(log_file, "r", encoding="utf-8") as file:
            for line in file:
                if "WARNING" in line:
                    warning_count += 1
                elif "ERROR" in line:
                    error_count += 1
                elif "CRITICAL" in line:
                    critical_count += 1
    except Exception as e:
        logging.error(f"Error reading log file {log_file}: {e}")

    return warning_count, error_count, critical_count

# Monitoring loop
monitoring_running = True  # Flag to control monitoring execution

def monitoring_loop():
    global monitoring_running
    while monitoring_running:
        if monitoring_enabled:
            time.sleep(MONITORING_INTERVAL)  # Wait for the interval
            notify_monitoring()
        time.sleep(1)  # Prevent CPU overuse

def stop_monitoring():
    global monitoring_running
    monitoring_running = False  # Stop monitoring when script exits

atexit.register(stop_monitoring)  # Ensure monitoring stops when the script exits

# Function to execute full library scan
def execute_full_library_scan():
    while True:
        clear_screen()
        print_banner()
        print("\nFull Library Scan Options:")
        print("1) Auto Scan")
        print("2) Auto Force Scan (Useful to recreate symlinks with auto select enabled)")
        print("3) Manual Scan (Use only when TMDB ID is enabled)")
        print("4) Manual Force Scan (Useful to recreate symlinks)")
        print("5) Back to Main Menu")
        choice = input("Select an option: ")

        scan_types = {
            '1': 'Auto Scan',
            '2': 'Auto Force Scan',
            '3': 'Manual Scan',
            '4': 'Manual Force Scan'
        }

        if choice in scan_types:
            notify_scan_started(scan_types[choice])

        try:
            if choice in ['1', '2', '3', '4']:
                if LIBRARY_SCRIPT and os.path.exists(LIBRARY_SCRIPT):
                    scan_args = {
                        '1': ['--auto-select'],
                        '2': ['--auto-select', '--force'],
                        '3': [],
                        '4': ['--force']
                    }
                    subprocess.run([python_command, LIBRARY_SCRIPT] + scan_args[choice], check=True)
                    input(f"{scan_types[choice]} completed. Press Enter to return to the main menu...")
                if not LIBRARY_SCRIPT or not os.path.exists(LIBRARY_SCRIPT):
                    print_color("Error: Library script not found!", "red")
                    logging.error("Library script not found. Cannot proceed with scan.")
                    return  # Exit function instead of running a broken subprocess
  
                else:
                    print_color("Error: The script does not exist.", "red")
                    logging.error("Library script path is invalid or does not exist.")
                    input("Press Enter to return to the main menu...")
            elif choice == '5':
                break
            else:
                print_color("Invalid option. Please select again.", "red")
                input("Press Enter to continue...")
        except subprocess.CalledProcessError as e:
            logging.error(f"Error executing library scan: {e}")
            print_color("Error executing scan. Check the log for details.", "red")

# Function to configure broken symlinks
def configure_broken_symlinks():
    config_file = os.path.join(BROKEN_LINK_FOLDER, "broken_links_config.txt")

    # Create the BrokenLinkVault folder if it doesn't exist
    os.makedirs(BROKEN_LINK_FOLDER, exist_ok=True)

    # Create the config file if it doesn't exist
    if not os.path.exists(config_file):
        open(config_file, 'w').close()

    while True:
        clear_screen()
        print_banner()
        print("\nRemove Broken Symlinks:")
        print("1) Run Scan")
        print("2) Add Directory")
        print("3) Remove Directory")
        print("4) Show Current Directories")
        print("5) Back to Main Menu")
        choice = input("Select an option: ")

        if choice == '1':
            execute_vault_scan()
        elif choice == '2':
            directory = input("Enter directory path to add: ").replace('\\', '/')
            if directory and os.path.isdir(directory):
                with open(config_file, 'r') as file:
                    if directory in file.read():
                        print_color("Directory already exists.", "yellow")
                    else:
                        with open(config_file, 'a') as file:
                            file.write(directory + '\n')
                        print_color("Directory added successfully.", "green")
                input("Press Enter to continue...")
            else:
                print_color("Invalid directory path. Please enter a valid directory.", "red")
                input("Press Enter to continue...")
        elif choice == '3':
            if os.path.getsize(config_file) == 0:
                print_color("No directories available.", "yellow")
                input("Press Enter to continue...")
                continue

            print("\nCurrent Directories:")
            with open(config_file, 'r') as file:
                for i, line in enumerate(file, 1):
                    print(f"{i}. {line.strip()}")

            try:
                index = int(input("Enter the number of the directory to remove: ")) - 1
                with open(config_file, 'r') as file:
                    lines = file.readlines()
                if 0 <= index < len(lines):
                    with open(config_file, 'w') as file:
                        for i, line in enumerate(lines):
                            if i != index:
                                file.write(line)
                    print_color("Directory removed successfully.", "green")
                else:
                    print_color("Invalid index. Please try again.", "red")
            except ValueError:
                print_color("Invalid input. Please enter a number.", "red")
            input("Press Enter to continue...")
        elif choice == '4':
            print("\nCurrent Directories:")
            with open(config_file, 'r') as file:
                if os.path.getsize(config_file) == 0:
                    print("No directories found.")
                else:
                    for i, line in enumerate(file, 1):
                        print(f"{i}. {line.strip()}")
            input("Press Enter to continue...")
        elif choice == '5':
            break
        else:
            print_color("Invalid option. Please select again.", "red")
            input("Press Enter to continue...")

# Function to execute vault scan
def execute_vault_scan():
    if os.path.exists(BROKEN_LINKS_SCRIPT):
        logging.info(f"Executing script at: {BROKEN_LINKS_SCRIPT}")
        try:
            result = subprocess.run([python_command, BROKEN_LINKS_SCRIPT], check=True, text=True, capture_output=True)
            print("Broken symlinks scan completed.")
            print("Output:\n", result.stdout)
            logging.info(f"Script output:\n{result.stdout}")
        except subprocess.CalledProcessError as e:
            logging.error(f"Error executing broken links script: {e.stderr}")
            print(f"Error executing scan. Check the log for details: {e.stderr}")
            logging.error(f"Command '{e.cmd}' returned non-zero exit status {e.returncode}.")
    else:
        print("Error: The script does not exist.")
    input("Press Enter to return to the main menu...")

# Function to execute Database Management
def database_management():
    while True:
        clear_screen()
        print_banner()
        print("\nDatabase Management Options:")
        print("1) View Database Status")
        print("2) Optimize Database")
        print("3) Verify Database Integrity")
        print("4) Vacuum Database")
        print("5) Export Database")
        print("6) Import Database")
        print("7) Search Database")
        print("8) Reset Database")
        print("9) Back to Main Menu")

        choice = input("Select an option: ")

        try:
            if choice == '1':
                stats = get_database_stats()
                if stats:
                    print("\nDatabase Statistics:")
                    print(f"Total Records: {stats['total_records']}")
                    print(f"Archived Records: {stats['archived_records']}")
                    print(f"Main DB Size: {stats['main_db_size']:.2f} MB")
                    print(f"Archive DB Size: {stats['archive_db_size']:.2f} MB")
            elif choice == '2':
                optimize_database()
            elif choice == '3':
                verify_database_integrity()
            elif choice == '4':
                vacuum_database()
            elif choice == '5':
                filename = input("Enter export filename (CSV): ")
                export_database(filename)
            elif choice == '6':
                filename = input("Enter import filename (CSV): ")
                import_database(filename)
            elif choice == '7':
                pattern = input("Enter search pattern: ")
                search_database(pattern)
            elif choice == '8':
                if input("Are you sure you want to reset the database? This will delete all entries. (Y/N): ").lower() == 'y':
                    reset_database()
            elif choice == '9':
                break
            else:
                print_color("Invalid option. Please select again.", "red")

            input("\nPress Enter to continue...")
        except Exception as e:
            logging.error(f"Error in database management: {e}")
            print_color(f"An error occurred: {e}", "red")
            input("\nPress Enter to continue...")

# Main function
def main():
    # Register script termination notification
    atexit.register(notify_script_terminated)

    # Start monitoring in the background
    if MONITORING_INTERVAL > 0:
        monitoring_thread = threading.Thread(target=monitoring_loop, daemon=True)
        monitoring_thread.start()

    while True:
        clear_screen()
        print_banner()
        greet_user()
        print("\nMain Menu:")
        print("1) Edit .env file")
        print("2) Full Library Scan")
        print("3) Configure Broken Symlinks")
        print("4) Database Management")
        print("5) RealTime-Monitoring Background Mode (Linux Only, Use NSSM for Windows)")
        print("6) Exit")

        choice = input("Select an option: ")

        if choice == '1':
            edit_env_file()
        elif choice == '2':
            execute_full_library_scan()
        elif choice == '3':
            configure_broken_symlinks()
        elif choice == '4':
            database_management()
        elif choice == '5':
            real_time_monitoring()
        elif choice == '6':
            print("Exiting the script. Have a great day!")
            break
        else:
            print_color("Invalid option. Please select again.", "red")
            input("Press Enter to continue...")

if __name__ == "__main__":
    main()

