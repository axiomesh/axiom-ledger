set -e

CURRENT_PATH=$(cd $(dirname ${BASH_SOURCE[0]}); pwd)
source ${CURRENT_PATH}/x.sh
source ${CURRENT_PATH}/smoke_env.sh
cd $CURRENT_PATH

function printHelp() {
    print_blue "Usage:  "
    echo "  smoke_test.sh [-b <BRANCH_NAME>] [-i <INTEGRATION_DIR>]"
    echo "  -'b' - the branch of base ref"
    echo "  -'i' - the integration test coverage directory"
    echo "  smoke_test.sh -h (print this message)"
}

function start_solo() {
    print_blue "===> 1. Start solo axiom-ledger"
    cd ../ && make build-coverage && cd scripts
    GOCOVERDIR=$INTEGRATION_DIR nohup bash solo.sh 2>error.log 1>solo.log &
}

function start_rbft() {
    print_blue "===> 1. Start rbft axiom-ledger"
    cd ../ && make build-coverage && cd scripts
    GOCOVERDIR=$INTEGRATION_DIR bash cluster.sh background
}

function start_tester() {
    print_blue "===> 2. Clone tester"
    echo "$BRANCH_NAME"
    cd ../
    git clone -b "$BRANCH_NAME" https://github.com/axiomesh/axiom-tester.git
    cd axiom-tester
    print_blue "===> 3. Start smoke test"
    npm install && npm run smoke-test
}

BRANCH_NAME="main"
while getopts ":h:b:i:" opt; do
  case "$opt" in
  h | \?)
    printHelp
    exit 0
    ;;
  b)
    BRANCH_NAME=$OPTARG
    ;;
  i)
    INTEGRATION_DIR=$OPTARG
    ;;
  esac
done

start_rbft
start_tester