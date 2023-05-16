package main

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
)

func main() {
	maxNum := 100
	secretNum := rand.Intn(maxNum)
	// fmt.Println("The secret number is:", secretNum)

	fmt.Println("Guess a number between 1 and", maxNum)
	guess := -1
	for guess != secretNum {
		reader := bufio.NewReader(os.Stdin)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println(err)
			continue
		}
		input = strings.TrimSpace(input)
		guess, err := strconv.Atoi(input)
		if err != nil {
			fmt.Println(err)
			continue
		}
		if guess < secretNum {
			fmt.Println("Too small!")
		} else if guess > secretNum {
			fmt.Println("Too big!")
		} else {
			fmt.Println("Correct!")
			return
		}
	}
}
