#! /bin/bash
set -e

base_dir=$(cd $(dirname ${BASH_SOURCE[0]}); pwd)

default_password="2025@draconis"

echo "Enter password (leave empty to use default password '$default_password'):"
read -s user_password

if [[ -z $user_password ]]; then
    # if password is empty, use default
    user_password=$default_password
fi

${base_dir}/draconis config generate --password $user_password
