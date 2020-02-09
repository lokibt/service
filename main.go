package main

import (
  "io"
  "bufio"
  "fmt"
  "net"
  "os"
  "strings"
  "crypto/rand"
  
  log "github.com/sirupsen/logrus"
)

var devices = make(map[string]map[string]net.Conn)
var connections = make(map[string]net.Conn)

func pseudoUuid() (uuid string) {
    b := make([]byte, 16)
    _, err := rand.Read(b)
    if err != nil {
        log.Info("Error: ", err)
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
      log.Info("Closing connection...")
      connection.Close()
    }
  }()

  log.Info("Handling connection...")
  reader := bufio.NewReader(connection)
  cmd := readTrimmedLine(reader)
  address := readTrimmedLine(reader)
  log.Info("Command " + cmd + " from: " + address)

  switch cmd {
    case "0":
      log.Info("Adding device...")
      devices[address] = make(map[string]net.Conn)

    case "1":
      log.Info("Removing device...")
      delete(devices, address)

    case "2":
      log.Info("Sending devices...")
      writer := bufio.NewWriter(connection)
      for addr, services := range devices {
        if addr != address {
          entry := addr
          for uuid, _ := range services {
            entry += "," + uuid
          }
          entry += "\n"
          log.Print(entry)
          writer.WriteString(entry)
        }
      }
      writer.Flush()

    case "3":
          log.Info("Adding service...")
          uuid := readTrimmedLine(reader)
          log.Info(uuid)
          devices[address][uuid] = connection
          log.Info("Keeping listener connection...")
          closeConn = false

    case "4":
          log.Info("Removing service...")
          uuid := readTrimmedLine(reader)
          log.Info(uuid)
          listenerConnection := devices[address][uuid]

          defer func() {
            log.Info("Closing listener connection...")
            listenerConnection.Close()
          }()

          delete(devices[address], uuid)

    case "5":
          log.Info("Establishing connection...")
          addr := readTrimmedLine(reader)
          log.Info(addr)
          uuid := readTrimmedLine(reader)
          log.Info(uuid)
          connId := pseudoUuid()
          log.Info(connId)
          writer := bufio.NewWriter(devices[addr][uuid])
          writer.WriteString(address + "\n")
          writer.WriteString(connId + "\n")
          writer.Flush()
          connections[connId] = connection
          log.Info("Keeping client connection...")
          closeConn = false

    case "6":
          log.Info("Linking connection...")
          connId := readTrimmedLine(reader)
          clientConnection := connections[connId]
          log.Info(connId)

          defer func() {
            log.Info("Closing client connection...")
            clientConnection.Close()
            delete(connections, connId)
          }()
          
          clientReader := bufio.NewReader(clientConnection)
          clientWriter := bufio.NewWriter(clientConnection)
          writer := bufio.NewWriter(connection)

          go func() {
            log.Info("Write from client to server...")
            clientReader.WriteTo(writer)
            log.Info("Write from client to server done.")
          }()
          
          log.Info("Write from server to client...")
          reader.WriteTo(clientWriter)
          log.Info("Write from server to client done.")
    
    default:
      log.Info("Unknown command...")
      for {
        line, err := reader.ReadString('\n')
        if err != nil && err != io.EOF {
          log.Info(err)
          break
        }
        log.Print(line)
      }
  }
}

func main() {
  if len(os.Args) == 1 {
    log.Info("Please provide a port number!")
    return
  }
  port := ":" + os.Args[1]

  listener, err := net.Listen("tcp4", port)
  if err != nil {
    log.Info(err)
    return
  }

  defer func() {
    log.Info("Stop listening...")
    listener.Close()
  }()

  for {
    log.Info("Waiting for connections...")
    connection, err := listener.Accept()
    if err != nil {
      log.Info(err)
      return
    }
    go handleConnection(connection)
  }
}     
