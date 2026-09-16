package main

import (
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"sync"
	"time"
	"unicode"

	"github.com/eiannone/keyboard"
	"github.com/hajimehoshi/ebiten/v2"
	//"encoding/binary"
)

var (
	keyMu      sync.Mutex
	lastPress  = make(map[rune]time.Time)
	keyHoldFor = 300 * time.Millisecond

	//Metadata
	behavior    string  = "old"  // desides if before the Shift command VX would be set or not (old - YES/new - NO)
	cpu_speed   float32 = 0.0007 //counted in MHz 0.0007
	timer_speed float32 = 0.00006
	// Registers
	reg_I       uint16                    // index register
	sound_timer uint8                     // sound timer register
	delay_timer uint8                     // delay timer register
	registers   []byte = make([]byte, 16) //general-purpose registers v0-vf

	//Memory
	rom []byte = make([]byte, 1024) // rom data buffer
	ram []byte = make([]byte, 4096) // ram memory
	//font data stored in first 512 bytes of ram
	font []byte = []byte{0xF0, 0x90, 0x90, 0x90, 0xF0,
		0x20, 0x60, 0x20, 0x20, 0x70,
		0xF0, 0x10, 0xF0, 0x80, 0xF0,
		0xF0, 0x10, 0xF0, 0x10, 0xF0,
		0x90, 0x90, 0xF0, 0x10, 0x10,
		0xF0, 0x80, 0xF0, 0x10, 0xF0,
		0xF0, 0x80, 0xF0, 0x90, 0xF0,
		0xF0, 0x10, 0x20, 0x40, 0x40,
		0xF0, 0x90, 0xF0, 0x90, 0xF0,
		0xF0, 0x90, 0xF0, 0x10, 0xF0,
		0xF0, 0x90, 0xF0, 0x90, 0x90,
		0xE0, 0x90, 0xE0, 0x90, 0xE0,
		0xF0, 0x80, 0x80, 0x80, 0xF0,
		0xE0, 0x90, 0x90, 0x90, 0xE0,
		0xF0, 0x80, 0xF0, 0x80, 0xF0,
		0xF0, 0x80, 0xF0, 0x80, 0x80}
	display [32][64]bool                    // Display 64x32 monochrome pixels
	stack   []uint16                        // stack
	keypad  []bool       = make([]bool, 16) // keypad data

	//Other
	display_dump [32][64]bool
	opcode       uint16              // opcode
	PC           int                 // opcode pointer
	rom_name     = "ROMs/Tetris.ch8" // name of executable rom
	sprite       byte                // container for the sprite data
	pixel        bool
)

type Game struct {
	display []byte
}

func startKeyboardListener() {
	if err := keyboard.Open(); err != nil {
		panic(err)
	}
	go func() {
		defer keyboard.Close()
		for {
			char, key, err := keyboard.GetKey()
			if err != nil {
				continue
			}
			if key == keyboard.KeyCtrlC {
				keyboard.Close()
				os.Exit(0)
			}
			keyMu.Lock()
			lastPress[unicode.ToLower(char)] = time.Now()
			keyMu.Unlock()
		}
	}()
}

func input_handler() {
	keyMu.Lock()
	defer keyMu.Unlock()
	now := time.Now()

	for i := range keypad {
		keypad[i] = false
	}

	held := func(r rune) bool {
		t, ok := lastPress[r]
		return ok && now.Sub(t) < keyHoldFor
	}

	if held('1') {
		keypad[0] = true
	}
	if held('2') {
		keypad[1] = true
	}
	if held('3') {
		keypad[2] = true
	}
	if held('4') {
		keypad[12] = true
	}

	if held('q') {
		keypad[3] = true
	}
	if held('w') {
		keypad[4] = true
	}
	if held('e') {
		keypad[5] = true
	}
	if held('r') {
		keypad[13] = true
	}

	if held('a') {
		keypad[6] = true
	}
	if held('s') {
		keypad[7] = true
	}
	if held('d') {
		keypad[8] = true
	}
	if held('f') {
		keypad[14] = true
	}

	if held('z') {
		keypad[9] = true
	}
	if held('x') {
		keypad[10] = true
	}
	if held('c') {
		keypad[11] = true
	}
	if held('v') {
		keypad[15] = true
	}
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
		stack = append(stack, uint16(PC))
		PC = int(opcode & 0x0FFF)
		PC -= 2
	case opcode == 0x00EE: // return from subroutine to the last adress in stack
		PC = int(stack[len(stack)-1])
		stack = stack[:len(stack)-1]
		//PC -= 2 //Every bug was due to this shit
	case opcode>>12 == 0x3: // 0x3XNN skip one instructrion if VX == NN
		if registers[opcode>>8&0x0F] == byte(opcode&0x00FF) {
			PC += 2
		}
	case opcode>>12 == 0x4: // 0x3XNN skip one instructrion if VX == NN
		if registers[opcode>>8&0x0F] != byte(opcode&0x00FF) {
			PC += 2
		}
	case opcode>>12 == 0x5: // 0x5XY0 skip one instructrion if VX == VY
		if registers[opcode>>8&0x0F] == registers[opcode>>4&0x00F] {
			PC += 2
		}
	case opcode>>12 == 0x6: // 0x6XNN set VX register to the value NN
		registers[opcode>>8&0x0F] = byte(opcode & 0x00FF)
	case opcode>>12 == 0x7: // 0x7XNN adds value NN to the VX register
		registers[opcode>>8&0x0F] += byte(opcode & 0x00FF)
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
			if int(registers[opcode>>8&0x0F])+int(registers[opcode>>4&0x00F]) >= 255 {
				registers[0xF] = 1
			} else {
				registers[0xF] = 0
			}
			registers[opcode>>8&0x0F] += registers[opcode>>4&0x00F]
		case 0x5: // sets VX to the result of VX - VY
			if registers[opcode>>8&0x0F] >= registers[opcode>>4&0x00F] {
				registers[0xF] = 1
			} else {
				registers[0xF] = 0
			}
			registers[opcode>>8&0x0F] -= registers[opcode>>4&0x00F]
		case 0x6: // Shift right
			if behavior == "old" {
				registers[opcode>>8&0x0F] = registers[opcode>>4&0x00F]
			}
			registers[opcode>>8&0x0F] = registers[opcode>>8&0x0F] >> 1
		case 0x7: // sets VX to the result of VY - VX
			if registers[opcode>>8&0x0F] <= registers[opcode>>4&0x00F] {
				registers[0xF] = 1
			} else {
				registers[0xF] = 0
			}
			registers[opcode>>8&0x0F] = registers[opcode>>4&0x00F] - registers[opcode>>8&0x0F]
		case 0xE: // Shift left
			if behavior == "old" {
				registers[opcode>>8&0x0F] = registers[opcode>>4&0x00F]
			}
			registers[opcode>>8&0x0F] = registers[opcode>>8&0x0F] << 1
		}
	case opcode>>12 == 0x9: // 0x5XY0 skip one instructrion if VX != VY
		if registers[opcode>>8&0x0F] != registers[opcode>>4&0x00F] {
			PC += 2
		}
	case opcode>>12 == 0xA: // Sets I (index) register
		reg_I = opcode & 0x0FFF
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
		X := registers[opcode>>8&0x0F] % 64
		Y := registers[opcode>>4&0x00F] % 32
		N := opcode & 0x000F
		registers[len(registers)-1] = 0
		for i := 0; i < int(N); i++ {
			sprite = ram[int(reg_I)+i]
			X = registers[opcode>>8&0x0F] % 64
			for j := 0; j < 8; j++ {
				pixel = (sprite>>(7-j))&1 == 1
				if X >= 64 {
					break
				}
				if pixel {
					if display[Y][X] {
						registers[len(registers)-1] = 1
					}
					display[Y][X] = !display[Y][X]
				}
				X++
			}
			Y++
			if Y >= 32 {
				break
			}
		}
	case opcode>>12 == 0xE: // 0xEX9E and 0xEXA1: Skip if key
		x := int(registers[opcode>>8&0x0F])
		if opcode&0x00FF == 0x9E { // skip if key VX is pressed
			if keypad[x] {
				PC += 2
			}
		}
		if opcode&0x00FF == 0xA1 { // skip if key VX is NOT pressed
			if !keypad[x] {
				PC += 2
			}
		}
	case opcode>>12 == 0xF: //FX07, FX15 and FX18: Timers
		switch opcode & 0x00FF {
		case 0x07:
			registers[opcode>>8&0x0F] = delay_timer
		case 0x0A:
			end := false
			for {
				time.Sleep(5 * time.Millisecond)
				if end {
					break
				}
				input_handler()
				for i := range len(keypad) {
					if keypad[i] {
						registers[opcode>>8&0x0F] = byte(i)
						end = true
						break
					}
				}
			}
		case 0x15:
			delay_timer = registers[opcode>>8&0x0F]
		case 0x18:
			sound_timer = registers[opcode>>8&0x0F]
		case 0x1E:
			reg_I += uint16(registers[opcode>>8&0x0F])
		case 0x29:
			reg_I = uint16(registers[opcode>>8&0x0F]) * 5
		case 0x33:
			x := (opcode >> 8) & 0x0F
			num := registers[x]
			if reg_I+2 < 4096 {
				ram[reg_I] = num / 100
				ram[reg_I+1] = (num / 10) % 10
				ram[reg_I+2] = num % 10
			}
		case 0x55:
			x := int((opcode >> 8) & 0x0F)
			for i := 0; i <= x; i++ {
				ram[int(reg_I)+i] = registers[i]
			}
		case 0x65:
			x := int((opcode >> 8) & 0x0F)
			for i := 0; i <= x; i++ {
				registers[i] = ram[int(reg_I)+i]
			}
		}
	default:
		fmt.Println("Nothing happened - ", opcode)
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
	fmt.Print("\033[H\033[2J")
	for _, row := range display {
		for _, pix := range row {
			if pix {
				fmt.Print("██")
			} else {
				fmt.Print("  ")
			}
		}
		fmt.Print("|\n")
	}
	fmt.Println("<><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><><>+")
	fmt.Println("-CHIP-8_EMU-")
	fmt.Println(registers)
	//fmt.Println(stack, " ", ram[stack[0]], " ", ram[stack[0]+1])
	fmt.Println(delay_timer, " ", sound_timer)
	for i := range keypad {
		if keypad[i] {
			fmt.Print("1 ")
		} else {
			fmt.Print("0 ")
		}
	}
	//fmt.Println("")

}

func start() {
	PC = 510 //Setting opcode pointer to the first memory rom bank

	//store font data into memory
	for i := range len(font) {
		ram[i] = font[i]
	}

	//read rom data and store into memory
	rom = readRom(rom_name)
	for i := range len(rom) {
		ram[i+512] = rom[i]
	}

	fmt.Println("Launched successfully!")
}

func loop() {
	cycleDuration := time.Second / time.Duration(cpu_speed*1_000_000)
	ticker := time.NewTicker(cycleDuration)
	defer ticker.Stop()
	for range ticker.C {
		disp_domp := display
		PC += 2
		if PC > 4096 || (uint16(ram[PC])<<8)|uint16(ram[PC+1]) == 0 {
			PC = 512
		}
		opcode = (uint16(ram[PC]) << 8) | uint16(ram[PC+1]) // getting curent opcode
		input_handler()
		cpu(opcode)
		for i := range len(display) {
			if disp_domp[i] != display[i] {
				render(display)
				break
			}
		}
	}
}

func frame_loop() {
	timerDuration := time.Second / time.Duration(timer_speed*1_000_000)
	ticker2 := time.NewTicker(timerDuration)
	Counter := 0
	for range ticker2.C {
		if Counter == 6 {
			Counter = 0
			render(display)
		}
		if delay_timer > 0 {
			delay_timer--
		}
		if sound_timer > 0 {
			sound_timer--
		}
		Counter++
	}
}

func (g *Game) Update() error {
	return nil
}

func (g *Game) Draw(pix []byte) {
	for _, i := range len(display) {
		for _, j := range len(display[i]) {
			if display[i][j] {
				pix[j] = 0xFF
			} else {
				pix[j] = 0x00
			}
		}
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	if g.pixels == nil {
		g.pixels = make([]byte, 320*240*4)
	}
	g.world.Draw(g.pixels)
	screen.WritePixels(g.pixels)
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return 320, 240
}

func main() {
	ebiten.SetWindowSize(64, 32)
	ebiten.SetWindowTitle("Game")
	go func() {
		if err := ebiten.RunGame(&Game{}); err != nil {
			log.Fatal(err)
		}
	}()
	startKeyboardListener()
	start()
	go frame_loop()
	loop()
}
