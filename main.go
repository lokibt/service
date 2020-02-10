package main

import (
  "io"
  "bufio"
  "fmt"
  "net"
  "os"
  "strings"
  "crypto/rand"
  "syscall"
  
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

// See https://stackoverflow.com/a/58664631
func connCheck(conn net.Conn) error {
    var sysErr error = nil
    rc, err := conn.(syscall.Conn).SyscallConn()
    if err != nil { return err }
    err = rc.Read(func(fd uintptr) bool {
        var buf []byte = []byte{0}
        n, _, err := syscall.Recvfrom(int(fd), buf, syscall.MSG_PEEK | syscall.MSG_DONTWAIT)
        switch {
        case n == 0 && err == nil:
            sysErr = io.EOF
        case err == syscall.EAGAIN || err == syscall.EWOULDBLOCK:
            sysErr = nil
        default:
            sysErr = err
        }
        return true
    })
    if err != nil { return err }

    return sysErr
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
  
  defer func() {
    log.Info("Closing connection...")
    connection.Close()
  }()

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

          defer func() {
            log.Info("Removing service...")
            delete(devices[address], uuid)
          }()

          log.Info("Keeping listener connection...")
          for {
            if (connCheck(connection) != nil) {
              log.Info("Bluetooth server closed listener connection.");
              break;
            }
          }

    /*case "4":
          log.Info("Removing service...")
          uuid := readTrimmedLine(reader)
          log.Info(uuid)
          listenerConnection := devices[address][uuid]

          defer func() {
            log.Info("Closing listener connection...")
            listenerConnection.Close()
          }()

          delete(devices[address], uuid)*/

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

          for {
            if (connCheck(connection) != nil) {
              log.Info("Bluetooth client closed connection");
              break;
            }
          }

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
            log.Info("Write from Bluetooth client to server...")
            clientReader.WriteTo(writer)
            log.Info("Write from Bluetooth client to server done.")
          }()
          
          go func() {
            log.Info("Write from Bluetooth server to client...")
            reader.WriteTo(clientWriter)
            log.Info("Write from Bluetooth server to client done.")
          }()

          for {
            if (connCheck(connection) != nil) {
              log.Info("Bluetooth server closed linked connection");
              break;
            }
            if (connCheck(clientConnection) != nil) {
              log.Info("Bluetooth client closed linked connection");
              break;
            }
          }
    
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

  log.Info("Start listening...")
  for {
    connection, err := listener.Accept()
    if err != nil {
      log.Info(err)
      return
    }
    go handleConnection(connection)
  }
}     
