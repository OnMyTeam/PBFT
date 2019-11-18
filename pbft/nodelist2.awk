#!/usr/bin/gawk -f

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
       for (i = 1; i <= (N+1)/2; i++) {
           if (i < N) {
               COMMA=","
           } else {
               COMMA=""
           }
           printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
           i, N1, portnumber + i, COMMA
       }
       for (i = (N+1)/2+1; i <= N+1; i++) {
           if (i < N) {
               COMMA=","
           } else {
               COMMA=""
           }
           printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
           i, N2, portnumber + i, COMMA
       }
       print "]"
   } else if (ENV == 2){ #AWS
         print "["
         for (i = 1; i <= (N+1)/7; i++) {
             if (i < N) {
                 COMMA=","
             } else {
                 COMMA=""
             }
             printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
             i, N1, portnumber + i, COMMA
         }
         for (i = (N+1)/2+1; i <= N+1; i++) {
             if (i < N) {
                 COMMA=","
             } else {
                 COMMA=""
             }
             printf "\t{\"nodeID\": \"Node%d\", \"url\": \"%s:%d\"}%s\n",
             i, N2, portnumber + i, COMMA
         }
         print "]"
   }

   if (N == 7){
	print "Hello!"
   } else if (N == 10) {
   } else if (N == 13) {
   } else if (N == 16) {
   } else if (N == 19) {
   } else if (N == 19) {
   } else if (N == 19) {
   } else if (N == 19) {
   } else if (N == 19) {
   } else if (N == 19) {
   } else if (N == 19) {
   } else if (N == 19) {
   } else if (N == 19) {
   } else if (N == 19) {
   } else if (N == 19) {
   }
}
