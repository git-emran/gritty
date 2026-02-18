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

if __name__ == "__main__":
    print_color_test()
    print_icon_test()
    print("Verification complete!")
