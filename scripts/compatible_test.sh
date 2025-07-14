#!/usr/bin/env bash
set -e
source x.sh
source smoke_env.sh

CURRENT_PATH=$(cd $(dirname ${BASH_SOURCE[0]}); pwd)
echo "CURRENT_PATH is: $CURRENT_PATH"
BRANCH_NAME="main"
NEW_TAG=$(git describe --tags $(git rev-list --tags --max-count=1))
OLD_TAG=$(git describe --tags $(git rev-list --tags --max-count=2) | grep -v "$NEW_TAG")
echo "Old tag: $OLD_TAG"
echo "New tag: $NEW_TAG"

function printHelp() {
    print_blue "Usage:  "
    echo "  smoke_test.sh [-b <BRANCH_NAME>] [-i <INTEGRATION_DIR>]"
    echo "  -'b' - the branch of base ref"
    echo "  -'i' - the integration test coverage directory"
    echo "  smoke_test.sh -h (print this message)"
}


function start_rbft() {
    print_blue "===> 1. Start axiom-ledger cluster with old tag"
    cd ../
    git checkout "$OLD_TAG" && make build 
    cd "$CURRENT_PATH"
    bash cluster.sh background
    print_green "$("$CURRENT_PATH"/build/node1/tools/bin/draconis version)"
}

function start_smoke-tester() {
    print_blue "===> 2. Clone and start smoke tester"
    cd "$CURRENT_PATH"
    git clone https://github.com/axiomesh/axiom-tester.git && cd axiom-tester
    git checkout "$BRANCH_NAME"
    npm install && npm run smoke-test
}

function replace_rbft_binary() {
    print_blue "===> 3. Replace rbft binary and srart cluster with new tag"
    cd "$CURRENT_PATH" && bash stop.sh
    cd ../ && git checkout "$NEW_TAG" && make build 
    for ((i = 1; i < 5; i = i + 1)); do
      mv scripts/build/node$i/tools/bin/draconis scripts/build/node$i/tools/bin/draconis.bak
      cp bin/draconis scripts/build/node$i/tools/bin/
      bash scripts/build/node$i/start.sh
    done
    print_green "$("$CURRENT_PATH"/build/node1/tools/bin/draconis version)"
}

function start_compatible-tester() {
    print_blue "===> 4. Start compatible test"
    cd "$CURRENT_PATH"/axiom-tester
    npm run compatible-test
}


start_rbft
sleep 10
start_smoke-tester
sleep 5
replace_rbft_binary
sleep 10
start_compatible-tester