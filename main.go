// INFO
// - You can change some variables in const list:
// 		- To change an executable rom, rename an "rom_name" variable
// 		- If the game run incorectly, you may try changing "behavior" variable to different state
// 		- You can increase game speed by insreasing "cpu_speed" variable (By default it's 0.0007MHz or 700Hz)
// -


package main

import (
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	//"encoding/binary"
)

//--Const list--
const (
	//--Metadata--
	rom_name     = "ROMs/B.ch8" // executable rom name
	behavior    string  = "old" // (old/new) desides how some instructions shoud work
	cpu_speed   float32 = 0.0007 // cpu speed (counted in MHz)
	timer_speed float32 = 0.00006 // frame loop fps (counted in MHz)

	//--Sound--
	sampleRate  = 44100
	beepFreq    = 440.0 // Emulated beep tone (default - 440Hz)
)

//--Variable list--
var (
	//--Registers--
	reg_I       uint16                    // index register
	sound_timer uint8                     // sound timer register
	delay_timer uint8                     // delay timer register
	registers   []byte = make([]byte, 16) //general-purpose registers v0-vf
	opcode       uint16              // operation code
	PC           int                 // opcode pointer

	//--Memory--
	rom []byte = make([]byte, 1024) // rom data buffer 1KB
	ram []byte = make([]byte, 4096) // ram memory 4KB
	// font data stored in first 512 bytes of ram
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
	display [32][64]bool // Display 64x32 monochrome pixels
	disp    []byte // display buffer for rendering
	stack   []uint16                    // stack
	keypad  []bool   = make([]bool, 16) // keypad data
	sprite       byte                // container for the sprite data
	pixel        bool
	// keypad key index map
	keyMap = map[ebiten.Key]int{
		ebiten.Key1: 0x1, ebiten.Key2: 0x2, ebiten.Key3: 0x3, ebiten.Key4: 0xC,
		ebiten.KeyQ: 0x4, ebiten.KeyW: 0x5, ebiten.KeyE: 0x6, ebiten.KeyR: 0xD,
		ebiten.KeyA: 0x7, ebiten.KeyS: 0x8, ebiten.KeyD: 0x9, ebiten.KeyF: 0xE,
		ebiten.KeyZ: 0xA, ebiten.KeyX: 0x0, ebiten.KeyC: 0xB, ebiten.KeyV: 0xF,
	}

	//Sound
	audioCtx    *audio.Context
	soundPlayer *audio.Player
)

type squareWave struct {
	freq float64
	pos  int64
}

type Game struct {
	display []byte
}



func (s *squareWave) Read(buf []byte) (int, error) {
	const amplitude = 6000
	period := int64(float64(sampleRate) / s.freq)
	if period <= 0 {
		period = 1
	}
	n := len(buf) / 4 // 4 bytes per stereo sample (2 bytes L + 2 bytes R, 16-bit)
	for i := 0; i < n; i++ {
		var v int16 = amplitude
		if s.pos%period >= period/2 {
			v = -amplitude
		}
		buf[4*i] = byte(v)
		buf[4*i+1] = byte(v >> 8)
		buf[4*i+2] = byte(v)
		buf[4*i+3] = byte(v >> 8)
		s.pos++
	}
	return n * 4, nil
}

func initAudio() {
	audioCtx = audio.NewContext(sampleRate)
	var err error
	soundPlayer, err = audioCtx.NewPlayer(&squareWave{freq: beepFreq})
	if err != nil {
		log.Fatal(err)
	}
}

// I thik I don't have to explain this
func input_handler() {
	for i := range keypad {
		keypad[i] = false
	}
	for key, idx := range keyMap {
		if ebiten.IsKeyPressed(key) {
			keypad[idx] = true
		}
	}
}

// cpu instruction interpratator
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

// reads a rom data from file and returns it as byte array
func readRom(name string) []byte {
	var result []byte

	result, err := os.ReadFile(name)
	if err != nil {
		panic(err)
	}

	return result
}

// initialisations command
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

// main cpu loop
func loop() {
	cycleDuration := time.Second / time.Duration(cpu_speed*1_000_000)
	ticker := time.NewTicker(cycleDuration)
	defer ticker.Stop()
	for range ticker.C {
		PC += 2
		if PC > 4096 || (uint16(ram[PC])<<8)|uint16(ram[PC+1]) == 0 {
			PC = 512
		}
		opcode = (uint16(ram[PC]) << 8) | uint16(ram[PC+1]) // getting curent opcode
		input_handler()
		cpu(opcode)

	}
}

// secondary loop for delay and sound timers
func frame_loop() {
	timerDuration := time.Second / time.Duration(timer_speed*1_000_000)
	ticker2 := time.NewTicker(timerDuration)
	Counter := 0
	for range ticker2.C {
		if delay_timer > 0 {
			delay_timer--
		}
		if sound_timer > 0 {
			sound_timer--
			if soundPlayer != nil && !soundPlayer.IsPlaying() {
				soundPlayer.Play()
			}else if soundPlayer != nil && soundPlayer.IsPlaying() {
				soundPlayer.Pause()
			}

		}
		Counter++
	}
}

// TF2 coconut picture ;)
func (g *Game) Update() error {
	return nil
}

// rendering display information
func (g *Game) Draw(screen *ebiten.Image) {
	disp = make([]byte, 0)
	for j := range display {
		for k := range display[j] {
			if display[j][k] {
				disp = append(disp, 255)
				disp = append(disp, 255)
				disp = append(disp, 255)
				disp = append(disp, 255)
			} else {
				disp = append(disp, 0)
				disp = append(disp, 0)
				disp = append(disp, 0)
				disp = append(disp, 0)

			}
		}
	}
	screen.WritePixels(disp)
}

//IDK
func (g *Game) Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int) {
	return 64, 32
}

func main() {
	ebiten.SetWindowSize(64*20, 32*20)
	ebiten.SetWindowTitle("Game")
	go func() {
		if err := ebiten.RunGame(&Game{}); err != nil {
			log.Fatal(err)
		}
	}()
	start()
	initAudio()
	go frame_loop()
	loop()
}
