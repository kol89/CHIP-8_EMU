package main

import (
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"time"

	"github.com/azul3d/keyboard"
	//"encoding/hex"
	//"encoding/binary"
)

var (
	//Metadata
	behavior  string  = "old" // desides if before the Shift command VX would be set or not (old - YES/new - NO)
	cpu_speed float32 = 1     //counted in MHz

	// Registers
	reg_I       uint16                    // index register
	sound_timer uint8                     // sound timer register
	delay_timer uint8                     // delay timer register
	registers   []byte = make([]byte, 16) //general-purpose registers v0-vf
	reg_dump    []byte = make([]byte, 16)

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
	display [32][64]bool                    // Display 64x32 monochrome pixels
	stack   []byte                          // stack
	keypad  []bool       = make([]bool, 16) // keypad data

	//Other
	ram_dump     []byte = make([]byte, 4096)
	display_dump [32][64]bool
	opcode       uint16      // opcode
	PC           int         // opcode pointer
	rom_name     = "IBM.ch8" // name of executable rom
	counter      int
	sprite       byte // container for the sprite data
	pixel        bool
	watcher      *keyboard.Watcher
)

func input_handler() {
	status := watcher.States()
	for i := range keypad {
		keypad[i] = false
	}
	for i := range keypad {
		keypad[i] = false
	}

	if status[keyboard.One] == keyboard.Down {
		keypad[0] = true
	}
	if status[keyboard.Two] == keyboard.Down {
		keypad[1] = true
	}
	if status[keyboard.Three] == keyboard.Down {
		keypad[2] = true
	}
	if status[keyboard.Four] == keyboard.Down {
		keypad[12] = true
	}

	if status[keyboard.Q] == keyboard.Down {
		keypad[3] = true
	}
	if status[keyboard.W] == keyboard.Down {
		keypad[4] = true
	}
	if status[keyboard.E] == keyboard.Down {
		keypad[5] = true
	}
	if status[keyboard.R] == keyboard.Down {
		keypad[13] = true
	}

	if status[keyboard.A] == keyboard.Down {
		keypad[6] = true
	}
	if status[keyboard.S] == keyboard.Down {
		keypad[7] = true
	}
	if status[keyboard.D] == keyboard.Down {
		keypad[8] = true
	}
	if status[keyboard.F] == keyboard.Down {
		keypad[14] = true
	}

	if status[keyboard.Z] == keyboard.Down {
		keypad[9] = true
	}
	if status[keyboard.X] == keyboard.Down {
		keypad[10] = true
	}
	if status[keyboard.C] == keyboard.Down {
		keypad[11] = true
	}
	if status[keyboard.V] == keyboard.Down {
		keypad[15] = true
	}
	for i := range keypad {
		if keypad[i] {
			fmt.Print("1")
		} else {
			fmt.Print("0")
		}
	}
	fmt.Println("")
}

func cpu(opcode uint16) {
	switch {
	case opcode == 0x00E0: // clear screen
		for i := range display {
			for j := range display[i] {
				display[i][j] = false
			}
		}
	case opcode>>12 == 0x1: // 0x1NNN jump to NNN
		PC = int(opcode & 0x0FFF)
		PC -= 2
	case opcode>>12 == 0x2: // 0x2NNN call a subroutine from NNN adress
		stack = append(stack, byte(PC))
		PC = int(opcode & 0x0FFF)
		PC -= 2
	case opcode == 0x00EE: // return from subroutine to the last adress in stack
		PC = int(stack[len(stack)-1])
		stack = stack[:len(stack)-1]
		PC -= 2
	case opcode>>12 == 0x3: // 0x3XNN skip one instructrion if VX == NN
		if registers[opcode>>8&0x0F] == byte(opcode&0x00FF) {
			PC += 2
		}
	case opcode>>12 == 0x5: // 0x5XY0 skip one instructrion if VX == VY
		if registers[opcode>>8&0x0F] == registers[opcode>>4&0x00F] {
			PC += 2
		}

	case opcode>>12 == 0x6: // 0x6XNN set VX register to the value NN
		registers[opcode>>8&0x0F] = byte(opcode & 0x00FF)
	case opcode>>12 == 0x7: // 0x7XNN adds value NN to the VX register
		if registers[opcode>>8&0x0F]+byte(opcode&0x00FF) > 0xFF {
			registers[opcode>>8&0x0F] = 0xFF
		} else {
			registers[opcode>>8&0x0F] += byte(opcode & 0x00FF)
		}
	case opcode>>12 == 0x8: // Logical and arithmetic instructions
		switch opcode & 0x000F {
		case 0x0: // Set
			registers[opcode>>8&0x0F] = registers[opcode>>4&0x00F]
		case 0x1: // Binary OR
			registers[opcode>>8&0x0F] = registers[opcode>>8&0x0F] | registers[opcode>>4&0x00F]
		case 0x2: // Binary AND
			registers[opcode>>8&0x0F] = registers[opcode>>8&0x0F] & registers[opcode>>4&0x00F]
		case 0x3: // Logical XOR
			registers[opcode>>8&0x0F] = registers[opcode>>8&0x0F] ^ registers[opcode>>4&0x00F]
		case 0x4: // Add
			registers[opcode>>8&0x0F] += registers[opcode>>4&0x00F]
		case 0x5: // sets VX to the result of VX - VY
			registers[opcode>>8&0x0F] -= registers[opcode>>4&0x00F]
		case 0x6: // Shift right
			if behavior == "old" {
				registers[opcode>>8&0x0F] = registers[opcode>>4&0x00F]
			}
			registers[opcode>>8&0x0F] = registers[opcode>>8&0x0F] >> 1
		case 0x7: // sets VX to the result of VY - VX
			registers[opcode>>8&0x0F] = registers[opcode>>4&0x00F] - registers[opcode>>8&0x0F]
		case 0xE: // Shift left
			if behavior == "old" {
				registers[opcode>>8&0x0F] = registers[opcode>>4&0x00F]
			}
			registers[opcode>>8&0x0F] = registers[opcode>>8&0x0F] << 1
		}

	case opcode>>12 == 0x9: // 0x5XY0 skip one instructrion if VX != VY
		if registers[opcode>>8&0x0F] != byte(opcode>>4&0x00F) {
			PC += 2
		}
	case opcode>>12 == 0xA: // Sets I (index) register
		reg_I = opcode & 0x00FF
	case opcode>>12 == 0xB: // 0xBNNN Jump with offset
		if behavior == "old" {
			PC = int(opcode&0x0FFF + uint16(registers[0]))
		} else {
			PC = int(opcode&0x0FFF + uint16(registers[opcode>>8&0x0F]))
		}
		PC -= 2
	case opcode>>12 == 0xC: // 0xCXNN Random
		registers[opcode>>8&0x0F] = byte(rand.Uint32()) & byte(opcode&0x00FF)
	case opcode>>12 == 0xD: // 0xDXYN Display WIP
		X := registers[opcode>>8&0x0F] & 63
		Y := registers[opcode>>4&0x00F] & 31
		registers[len(registers)-1] = 0
		for i := 0; i <= int(opcode&0x000F); i++ {
			sprite = ram[reg_I+(opcode&0x000F)]
			for j := 0; j <= 8; j++ {
				pixel = (sprite>>(7-j))&1 == 1
				if X > 64 {
					break
				}
				if pixel {
					display[Y][X] = !display[Y][X]
					registers[len(registers)-1] = 1
				}
				X++
			}
			Y++
			if Y > 32 {
				break
			}
		}
	case opcode>>12 == 0xF:
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
	for {
		for _, row := range display {
			for _, pix := range row {
				if pix {
					fmt.Print("#")
				} else {
					fmt.Print(".")
				}
			}
			fmt.Print("\n")
		}
	}
}

func start() {
	PC = 512 //Setting opcode pointer to the first memory rom bank
	counter = 0

	//store font data into memory
	for i := 0; i < len(font); i++ {
		ram[i] = font[i]
	}

	//read rom data and store into memory
	rom = readRom(rom_name)
	for i := 0; i < len(rom); i++ {
		ram[i+512] = rom[i]
	}

	fmt.Println("Launched successfully!")
}

func loop() {
	cycleDuration := time.Second / time.Duration(cpu_speed*1_000_000)
	ticker := time.NewTicker(cycleDuration)
	defer ticker.Stop()
	for range ticker.C {
		ram_dump = ram
		reg_dump = registers
		PC += 2
		counter++
		if PC >= 1536 {
			PC = 512
		}
		opcode = (uint16(ram[PC+1]) << 8) | uint16(ram[PC]) // getting curent opcode
		input_handler()
		cpu(opcode)
		if !slices.Equal(ram, ram_dump) || !slices.Equal(registers, reg_dump) {
			fmt.Println(ram)
			//render(display)
		}
		//fmt.Println(ram[PC], "/", ram[PC+1])
		if counter == 700 {
			counter = 0
			// fmt.Println("\n")

			//render(display)
			//render(display)
			//fmt.Println("////")
			//fmt.Println(ram)
		}
	}
}

func main() {
	//go render(display)
	watcher = keyboard.NewWatcher()
	start()
	loop()
}
