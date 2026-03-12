#!/bin/sh
export VENDOR_STUB_PORT="${PORT:-8081}"
exec ./vendorstub
