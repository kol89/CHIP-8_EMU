package main

import (
	"os"
	"fmt"
	"time"
	"slices"
	//"encoding/hex"
	//"encoding/binary"
)


var (
	//Metadata
	cpu_speed float32 = 0.0007 //counted in MHz

	// Registers
	reg_I uint16 // index register
	sound_timer uint8 // sound timer register
	delay_timer uint8 // delay timer register
	registers []byte = make([]byte, 16)//general-purpose registers v0-vf

	//Memory
	rom []byte = make([]byte, 1024) // rom data buffer
	ram []byte = make([]byte, 4096) // ram memory
	//font data stored in first 512 bytes of ram
	font []byte = []byte{
		0xF0, 0x90, 0x90, 0x90, 0xF0, // 0
		0x20, 0x60, 0x20, 0x20, 0x70, // 1
		0xF0, 0x10, 0xF0, 0x80, 0xF0, // 2
		0xF0, 0x10, 0xF0, 0x10, 0xF0, // 3
		0x90, 0x90, 0xF0, 0x10, 0x10, // 4
		0xF0, 0x80, 0xF0, 0x10, 0xF0, // 5
		0xF0, 0x80, 0xF0, 0x90, 0xF0, // 6
		0xF0, 0x10, 0x20, 0x40, 0x40, // 7
		0xF0, 0x90, 0xF0, 0x90, 0xF0, // 8
		0xF0, 0x90, 0xF0, 0x10, 0xF0, // 9
		0xF0, 0x90, 0xF0, 0x90, 0x90, // A
		0xE0, 0x90, 0xE0, 0x90, 0xE0, // B
		0xF0, 0x80, 0x80, 0x80, 0xF0, // C
		0xE0, 0x90, 0x90, 0x90, 0xE0, // D
		0xF0, 0x80, 0xF0, 0x80, 0xF0, // E
		0xF0, 0x80, 0xF0, 0x80, 0x80} // F
	display [32][64]bool // Display 64x32 monochrome pixels
	stack []byte // stack

	//Other
	ram_dump []byte = make([]byte, 4096)
	display_dump [32][64]bool
	opcode uint16 // opcode
	PC int // opcode pointer
	rom_name = "IBM.ch8" // name of executable rom
	counter int
)

func cpu(opcode uint16){
	switch{
		case opcode == 0x00E0: // clear screen
			for i := range display {
				for j := range display[i] {
					display[i][j] = false
				}
			}
		case opcode>>12 == 0x1: // 0x1NNN jump to NNN
			PC = int(opcode & 0x0FFF)
			PC-=2
		case opcode>>12 == 0x2: // 0x2NNN call a subroutine from NNN adress
			stack = append(stack, byte(PC))
			PC = int(opcode & 0x0FFF)
			PC-=2
		case opcode == 0x00EE: // return from subroutine to the last adress in stack
			PC = int(stack[len(stack)-1])
			stack = stack[:len(stack)-1]
			PC-=2
		case opcode>>12 == 0x3: // 0x3XNN skip one instructrion if vX == NN
			if registers[opcode>>8 & 0x0F] == byte(opcode & 0x00FF){
				PC+=2
			}
		case opcode>>12 == 0x5: // 0x5XY0 skip one instructrion if vX == vY
			if registers[opcode>>8 & 0x0F] == byte(opcode>>4 & 0x00F){
				PC+=2
			}
		
		case opcode>>12 == 0x6:
		case opcode>>12 == 0x7:
		case opcode>>12 == 0x9: // 0x5XY0 skip one instructrion if vX != vY
			if registers[opcode>>8 & 0x0F] != byte(opcode>>4 & 0x00F){
				PC+=2
			}
	}
}

func readRom(name string) []byte {
	var result []byte

	result, err := os.ReadFile(name)
	if err != nil {
		panic(err)
	}

	return result
}

func render(display [32][64]bool) {
	for _ , row := range display {
		for _ , pix := range row{
			if pix{
				fmt.Print("#")
			}else{
				fmt.Print(".")
			}
		}
		fmt.Print("\n")
	}
}

func start() {
	PC = 512 //Setting opcode pointer to the first memory rom bank
	counter = 0

	//store font data into memory
	for i:=0; i<len(font); i++ {
		ram[i]=font[i]
	}

	//read rom data and store into memory
	rom = readRom(rom_name)
	for i:=0; i<len(rom); i++ {
		ram[i+512]=rom[i]
	}

	fmt.Println("Launched successfully!")
}

func loop() {
	cycleDuration := time.Second / time.Duration(cpu_speed*1_000_000)
	ticker := time.NewTicker(cycleDuration)
	defer ticker.Stop()

	for range ticker.C {
		ram_dump = ram
		PC+=2
		counter++
		if PC >= 1536{
			PC = 512
		}
		opcode = (uint16(ram[PC+1]) << 8) | uint16(ram[PC]) // getting curent opcode
		cpu(opcode)
		if !slices.Equal(ram, ram_dump) || display_dump!=display{
			render(display)
		}
		//fmt.Println(ram[PC], "/", ram[PC+1])
		if counter==700{
			counter = 0
			//fmt.Println("////")
			//fmt.Println(ram)
		}
	}
}

func main(){
	start()
	loop()
}