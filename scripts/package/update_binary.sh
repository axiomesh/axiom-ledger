#! /bin/bash
set -e

if [ -z "$1" ]; then
    echo "error: new binary path is empty"
    echo "Usage: ./update_binary.sh new_binary_path"
    exit 1
fi

if [ ! -f $1 ]; then
  echo "error: new binary($1) does not exist"
  echo "Usage: ./update_binary.sh new_binary_path"
  exit 1
fi

base_dir=$(cd $(dirname ${BASH_SOURCE[0]}); pwd)

# backup old binary
old_binary=${base_dir}/tools/bin/draconis-$(date +%Y-%m-%d-%H-%M-%S).bak
cp -f ${base_dir}/tools/bin/draconis ${old_binary}
cp -f $1 ${base_dir}/tools/bin/draconis

echo "backup old binary to ${old_binary}"
echo "new binary:"
${base_dir}/draconis version