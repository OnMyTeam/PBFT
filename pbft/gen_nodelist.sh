#!/bin/bash
RED='\033[0;31m'
NC='\033[0m'
if [[ $# -lt 1 ]]
then
	echo "Usage: $0 <# of nodes> <environment> <N1 ip> <N2 ip> <N3 ip> <N4 ip> <N5 ip> <N6 ip> <N7 ip>"
	echo "Example: $0 43 2 \"192.168.1.1\"\"192.168.1.1\"\"192.168.1.1\"\"192.168.1.1\"\"192.168.1.1\"\"192.168.1.1\"\"192.168.1.1\""
	exit
fi

TOTALNODE=$1
if [ $2 -eq 0 ];then
	echo "local"
	NODELISTPATH="./seedList/nodeNum"$TOTALNODE"/nodeList.json"
  echo `awk -v N=$1 -v ENV=$2 -v N1=$3 -v N2=$4 -v N3=$5 -v N4=$6 -v N5=$7 -v N6=$8 -v N7=$9 -f nodelist2.awk /dev/null` > $NODELISTPATH
	exit
fi
if [ $2 -eq 1 ];then
	echo "remote"
	NODELISTPATH="./seedList/nodeNum"$TOTALNODE"/nodeList2_"$TOTALNODE".json"
  echo `awk -v N=$1 -v ENV=$2 -v N1=$3 -v N2=$4 -v N3=$5 -v N4=$6 -v N5=$7 -v N6=$8 -v N7=$9 -f nodelist2.awk /dev/null` > $NODELISTPATH
	exit
fi
if [ $2 -eq 2 ];then
	echo "aws"
	NODELISTPATH="./seedList/nodeNum"$TOTALNODE"/nodeList_aws.json"
  echo `awk -v N=$1 -v ENV=$2 -v N1=$3 -v N2=$4 -v N3=$5 -v N4=$6 -v N5=$7 -v N6=$8 -v N7=$9 -f nodelist2.awk /dev/null` > $NODELISTPATH
  exit
fi

wait
