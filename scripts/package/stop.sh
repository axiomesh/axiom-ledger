#! /bin/bash
set -e

base_dir=$(cd $(dirname ${BASH_SOURCE[0]}); pwd)
env_file=${base_dir}/.env
if [ -f ${env_file} ]; then
  export DRAGON_COINS_ENV_FILE=${env_file}
fi
export DRAGON_COINS_PATH=${base_dir}

${base_dir}/tools/control.sh stop