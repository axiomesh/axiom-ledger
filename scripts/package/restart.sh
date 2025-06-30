#! /bin/bash
set -e

base_dir=$(cd $(dirname ${BASH_SOURCE[0]}); pwd)
env_file=${base_dir}/.env
if [ -f ${env_file} ]; then
  export DRACONIS_ENV_FILE=${env_file}
fi
export DRACONIS_PATH=${base_dir}

${base_dir}/tools/control.sh restart