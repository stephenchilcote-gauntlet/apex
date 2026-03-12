#!/usr/bin/env python3
"""
Playwright ffmpeg wrapper — replaces VP8 with h264_nvenc (NVENC hardware encoding).

Playwright hardcodes: -c:v vp8 -qmin 0 -qmax 50 -crf 8 -deadline realtime -speed 8 -b:v 1M
This wrapper substitutes:  -c:v h264_nvenc -preset p4 -cq 20 -pix_fmt yuv420p

Install:
  FFMPEG=~/.cache/ms-playwright/ffmpeg-1011/ffmpeg-linux
  cp $FFMPEG ${FFMPEG}-vp8-original
  cp tests/e2e/ffmpeg-nvenc-wrapper.py $FFMPEG
  chmod +x $FFMPEG
"""
import sys
import os

args = list(sys.argv[1:])
new_args = []
i = 0
while i < len(args):
    a = args[i]

    # Replace vp8 codec with h264_nvenc
    if a == '-c:v' and i + 1 < len(args) and args[i + 1] == 'vp8':
        new_args += ['-c:v', 'h264_nvenc']
        i += 2
        continue

    # Drop VP8-only quality flags
    if a in ('-qmin', '-qmax', '-crf', '-deadline', '-speed') and i + 1 < len(args):
        i += 2  # skip flag + value
        continue

    # Replace VP8 bitrate cap with NVENC quality settings
    if a == '-b:v' and i + 1 < len(args) and args[i + 1] == '1M':
        new_args += ['-preset', 'p4', '-cq', '20', '-pix_fmt', 'yuv420p']
        i += 2
        continue

    # Drop single-thread restriction (unnecessary for NVENC)
    if a == '-threads' and i + 1 < len(args) and args[i + 1] == '1':
        i += 2
        continue

    new_args.append(a)
    i += 1

os.execvp('/usr/bin/ffmpeg', ['/usr/bin/ffmpeg'] + new_args)
