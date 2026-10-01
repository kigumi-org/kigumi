#!/bin/sh
# gen --value N --out FILE: writes a C function returning N.
printf 'int gen_value(void) { return %s; }\n' "$2" > "$4"
