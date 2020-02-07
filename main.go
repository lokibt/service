package main

import (
  "io"
  "bufio"
  "fmt"
  "net"
  "os"
  "strings"
  "crypto/rand"
)

var devices = make(map[string]map[string]net.Conn)
var connections = make(map[string]net.Conn)

func pseudoUuid() (uuid string) {
    b := make([]byte, 16)
    _, err := rand.Read(b)
    if err != nil {
        fmt.Println("Error: ", err)
        return
    }

    uuid = fmt.Sprintf("%X-%X-%X-%X-%X", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])

    return
}

func readTrimmedLine(reader *bufio.Reader) string {
    line, err := reader.ReadString('\n')
    if err != nil {
      panic(err)
    }
    line = strings.TrimSpace(string(line))
    return line
}

func handleConnection(connection net.Conn) {
  closeConn := true
  
  defer func() {
    if closeConn {
      fmt.Println("Closing connection...")
      connection.Close()
    }
  }()

  fmt.Println("Handling connection...")
  reader := bufio.NewReader(connection)
  cmd := readTrimmedLine(reader)
  address := readTrimmedLine(reader)
  fmt.Println("Command " + cmd + " from: " + address)

  switch cmd {
    case "0":
      fmt.Println("Adding device...")
      devices[address] = make(map[string]net.Conn)

    case "1":
      fmt.Println("Removing device...")
      delete(devices, address)

    case "2":
      fmt.Println("Sending devices...")
      writer := bufio.NewWriter(connection)
      for addr, services := range devices {
        if addr != address {
          entry := addr
          for uuid, _ := range services {
            entry += "," + uuid
          }
          entry += "\n"
          fmt.Print(entry)
          writer.WriteString(entry)
        }
      }
      writer.Flush()

    case "3":
          fmt.Println("Adding service...")
          uuid := readTrimmedLine(reader)
          fmt.Println(uuid)
          devices[address][uuid] = connection
          closeConn = false

    case "4":
          fmt.Println("Removing service...")
          uuid := readTrimmedLine(reader)
          fmt.Println(uuid)
          listenerConnection := devices[address][uuid]

          defer func() {
            fmt.Println("Closing client connection...")
            listenerConnection.Close()
          }()

          delete(devices[address], uuid)

    case "5":
          fmt.Println("Establishing connection...")
          addr := readTrimmedLine(reader)
          fmt.Println(addr)
          uuid := readTrimmedLine(reader)
          fmt.Println(uuid)
          connId := pseudoUuid()
          fmt.Println(connId)
          writer := bufio.NewWriter(devices[addr][uuid])
          writer.WriteString(address + "\n")
          writer.WriteString(connId + "\n")
          writer.Flush()
          connections[connId] = connection
          closeConn = false

    case "6":
          fmt.Println("Linking connection...")
          connId := readTrimmedLine(reader)
          clientConnection := connections[connId]
          fmt.Println(connId)

          defer func() {
            fmt.Println("Closing client connection...")
            clientConnection.Close()
            delete(connections, connId)
          }()
          
          clientReader := bufio.NewReader(clientConnection)
          clientWriter := bufio.NewWriter(clientConnection)
          writer := bufio.NewWriter(connection)

          go func() {
            fmt.Println("Write from client to server...")
            clientReader.WriteTo(writer)
            fmt.Println("Write from client to server done.")
          }()
          
          fmt.Println("Write from server to client...")
          reader.WriteTo(clientWriter)
          fmt.Println("Write from server to client done.")
    
    default:
      fmt.Println("Unknown command...")
      for {
        line, err := reader.ReadString('\n')
        if err != nil && err != io.EOF {
          fmt.Println(err)
          break
        }
        fmt.Print(line)
      }
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

  defer func() {
    fmt.Println("Stop listening...")
    listener.Close()
  }()

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
