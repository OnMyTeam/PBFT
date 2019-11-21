#!/usr/bin/gawk -f
function printing(start, end, ip){

  return
}
BEGIN {
   # User input
   # N: number of nodes

   portnumber = 1110

}

END {

   if (ENV == 0) {  #LOCAL
       print "["
       for (i = 1; i <= N; i++) {
          if (i < N) {
              COMMA=","
          } else {
              COMMA=""
          }
          printf "\t{\"nodeID\": \"Node%d\", \"url\": \"localhost:%d\"}%s\n",
          i, portnumber + i, COMMA
       }
       print "]"
   } else if (ENV == 1){ #REMOTE
       print "["
       for (i = 1; i <= (N+2)/3; i++) {
           if (i < N) {
               COMMA=","
           } else {
               COMMA=""
           }
           printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
           i, N1, portnumber + i, COMMA
       }
       for (i = (N+2)/3+1; i <= (N*2+1)/3; i++) {
           if (i < N) {
               COMMA=","
           } else {
               COMMA=""
           }
           printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
           i, N2, portnumber + i, COMMA
       }
       for (i = (N*2+1)/3+1; i <= N; i++) {
           if (i < N) {
               COMMA=","
           } else {
               COMMA=""
           }
           printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
           i, N3, portnumber + i, COMMA
       }
       print "]"
   } else if (ENV == 2){ #AWS
         if (N == 7) {
           start1 = 1
           end1 = 1
           start2 = 2
           end2 = 2
           start3 = 3
           end3 = 3
           start4 = 4
           end4 = 4
           start5 = 5
           end5 = 5
           start6 = 6
           end6 = 6
           start7 = 7
           end7 = 7
         } else if (N==10){
           start1 = 1
           end1 = 2
           start2 = 3
           end2 = 4
           start3 = 5
           end3 = 6
           start4 = 7
           end4 = 7
           start5 = 8
           end5 = 8
           start6 = 9
           end6 = 9
           start7 = 10
           end7 = 10
         } else if (N==13){
           start1 = 1
           end1 = 5
           start2 = 6
           end2 = 7
           start3 = 8
           end3 = 9
           start4 = 10
           end4 = 10
           start5 = 11
           end5 = 11
           start6 = 12
           end6 = 12
           start7 = 13
           end7 = 13
         } else if (N==16){
           start1 = 1
           end1 = 3
           start2 = 4
           end2 = 6
           start3 = 7
           end3 = 8
           start4 = 9
           end4 = 10
           start5 = 11
           end5 = 12
           start6 = 13
           end6 = 14
           start7 = 15
           end7 = 16
         } else if (N==19){
           start1 = 1
           end1 = 6
           start2 = 7
           end2 = 9
           start3 = 10
           end3 = 11
           start4 = 12
           end4 = 13
           start5 = 14
           end5 = 15
           start6 = 16
           end6 = 17
           start7 = 18
           end7 = 19
         } else if (N==22){
           start1 = 1
           end1 = 4
           start2 = 5
           end2 = 7
           start3 = 8
           end3 = 10
           start4 = 11
           end4 = 13
           start5 = 14
           end5 = 16
           start6 = 17
           end6 = 19
           start7 = 20
           end7 = 22
         } else if (N==25){
           start1 = 1
           end1 = 4
           start2 = 5
           end2 = 8
           start3 = 9
           end3 = 12
           start4 = 13
           end4 = 16
           start5 = 17
           end5 = 19
           start6 = 20
           end6 = 22
           start7 = 23
           end7 = 25
         } else if (N==28){
           start1 = 1
           end1 = 4
           start2 = 5
           end2 = 8
           start3 = 9
           end3 = 12
           start4 = 13
           end4 = 16
           start5 = 17
           end5 = 20
           start6 = 21
           end6 = 24
           start7 = 25
           end7 = 28
         } else if (N==31){
           start1 = 1
           end1 = 5
           start2 = 6
           end2 = 10
           start3 = 11
           end3 = 15
           start4 = 16
           end4 = 19
           start5 = 20
           end5 = 23
           start6 = 24
           end6 = 27
           start7 = 28
           end7 = 31
         } else if (N==34){
           start1 = 1
           end1 = 5
           start2 = 6
           end2 = 10
           start3 = 11
           end3 = 15
           start4 = 16
           end4 = 20
           start5 = 21
           end5 = 25
           start6 = 26
           end6 = 30
           start7 = 31
           end7 = 34
         } else if (N==37){
           start1 = 1
           end1 = 6
           start2 = 7
           end2 = 12
           start3 = 13
           end3 = 17
           start4 = 18
           end4 = 22
           start5 = 23
           end5 = 27
           start6 = 28
           end6 = 32
           start7 = 33
           end7 = 37
         } else if (N==40){
           start1 = 1
           end1 = 6
           start2 = 7
           end2 = 12
           start3 = 13
           end3 = 18
           start4 = 19
           end4 = 24
           start5 = 25
           end5 = 30
           start6 = 31
           end6 = 35
           start7 = 36
           end7 = 40
         } else if (N==43){
           start1 = 1
           end1 = 7
           start2 = 8
           end2 = 13
           start3 = 14
           end3 = 19
           start4 = 20
           end4 = 25
           start5 = 26
           end5 = 31
           start6 = 32
           end6 = 37
           start7 = 38
           end7 = 43
         }
         print "["
         for (i = start1; i <= end1; i++) {
             if (i < N) {
                 COMMA=","
             } else {
                 COMMA=""
             }
             printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
             i, N1, portnumber + i, COMMA
         }
         for (i = start2; i <= end2; i++) {
             if (i < N) {
                 COMMA=","
             } else {
                 COMMA=""
             }
             printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
             i, N2, portnumber + i, COMMA
         }
         for (i = start3; i <= end3; i++) {
             if (i < N) {
                 COMMA=","
             } else {
                 COMMA=""
             }
             printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
             i, N3, portnumber + i, COMMA
         }
         for (i = start4; i <= end4; i++) {
             if (i < N) {
                 COMMA=","
             } else {
                 COMMA=""
             }
             printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
             i, N4, portnumber + i, COMMA
         }
         for (i = start5; i <= end5; i++) {
             if (i < N) {
                 COMMA=","
             } else {
                 COMMA=""
             }
             printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
             i, N5, portnumber + i, COMMA
         }
         for (i = start6; i <= end6; i++) {
             if (i < N) {
                 COMMA=","
             } else {
                 COMMA=""
             }
             printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
             i, N6, portnumber + i, COMMA
         }
         for (i = start7; i <= end7; i++) {
             if (i < N) {
                 COMMA=","
             } else {
                 COMMA=""
             }
             printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
             i, N7, portnumber + i, COMMA
         }
         print "]"
   }
}

