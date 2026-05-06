#!/usr/bin/env python3
import sys

def print_color_test():
    print("--- 16 ANSI Colors ---")
    for i in range(16):
        print(f"\033[48;5;{i}m  \033[0m", end="")
        if (i + 1) % 8 == 0:
            print()
    print()

    print("--- 256 Color Cube ---")
    for i in range(16, 232):
        print(f"\033[48;5;{i}m \033[0m", end="")
        if (i - 15) % 36 == 0:
            print()
    print()

    print("--- Grayscale Ramp ---")
    for i in range(232, 256):
        print(f"\033[48;5;{i}m \033[0m", end="")
    print("\n")

    print("--- TrueColor (RGB) Gradient ---")
    for i in range(0, 256, 16):
        print(f"\033[48;2;{i};0;0m \033[0m", end="")
    print()
    for i in range(0, 256, 16):
        print(f"\033[48;2;0;{i};0m \033[0m", end="")
    print()
    for i in range(0, 256, 16):
        print(f"\033[48;2;0;0;{i}m \033[0m", end="")
    print("\n")

def print_icon_test():
    print("--- Icons (Nerd Font) ---")
    icons = [
        ("\uf120", "Terminal"),
        ("\uf17a", "Windows"),
        ("\uf179", "Apple"),
        ("\uf113", "GitHub"),
        ("\ue712", "Python"),
        ("\ue627", "Go"),
        ("\ue708", "Docker"),
    ]
    for icon, name in icons:
        print(f"{icon}  {name}")
    print()

    print("--- Wide Characters (Emoji) ---")
    emojis = ["🚀", "🔥", "🌈", "💻", "⚡"]
    for e in emojis:
        print(f"{e}  ", end="")
    print("\n")

def print_style_test():
    print("--- Styles (Italic / Underline) ---")
    print("normal  " + "\033[3mitalic\033[0m  " + "\033[4munderline\033[0m  " + "\033[1mbold\033[0m")
    print()

def print_scroll_region_test():
    print("--- Scrolling Region (DECSTBM) ---")
    # Draw a boxed region and then scroll inside it.
    # Region: rows 3..8 (1-based)
    print("line 1 (outside)")
    print("line 2 (outside)")
    print("\033[3;8r", end="")   # set scroll region
    print("\033[3;1H", end="")   # go to top of region
    for i in range(1, 12):
        print(f"region line {i:02d}")
    print("\033[r", end="")      # reset scroll region to full
    print("\nline 9 (outside)")
    print("line 10 (outside)")
    print()

def print_alt_screen_test():
    print("--- Alternate Screen (?1049) ---")
    print("Switching to alt screen for a moment...")
    print("\033[?1049h", end="")  # enter alt screen
    print("\033[2J\033[H", end="")
    print("ALT SCREEN: if you see this, alt-screen works.")
    print("Returning to main screen in 1s...")
    sys.stdout.flush()
    try:
        import time
        time.sleep(1)
    except Exception:
        pass
    print("\033[?1049l", end="")  # exit alt screen
    print("Back on main screen.")
    print()

if __name__ == "__main__":
    print_color_test()
    print_style_test()
    print_scroll_region_test()
    print_alt_screen_test()
    print_icon_test()
    print("Verification complete!")
