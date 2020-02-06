package main

import (
  "bufio"
  "fmt"
  "net"
  "os"
)

func closeConnection(connection net.Conn) {
  fmt.Println("Closing connection...")
  connection.Close()
}

func handleConnection(connection net.Conn) {
  defer closeConnection(connection)
  
  fmt.Println("Handling connection...")
  reader := bufio.NewReader(connection)

  for {
    data, err := reader.ReadString('\n')
    if err != nil {
      fmt.Println(err)
      return
    }
    fmt.Print(string(data))
  }
}

func main() {
  if len(os.Args) == 1 {
    fmt.Println("Please provide a port number!")
    return
  }
  port := ":" + os.Args[1]

  listener, err := net.Listen("tcp4", port)
  if err != nil {
    fmt.Println(err)
    return
  }
  defer listener.Close()

  for {
    fmt.Println("Waiting for connections...")
    connection, err := listener.Accept()
    if err != nil {
      fmt.Println(err)
      return
    }
    go handleConnection(connection)
  }
}     
