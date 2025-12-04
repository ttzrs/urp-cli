// P2: Código de prueba para análisis
package main

import (
	"fmt"
	"os"
)

// User representa un usuario del sistema
type User struct {
	ID    int
	Name  string
	Email string
}

// GetUser obtiene un usuario por ID
func GetUser(id int) (*User, error) {
	// Simulación de búsqueda
	if id <= 0 {
		return nil, fmt.Errorf("invalid id: %d", id)
	}
	return &User{ID: id, Name: "Test", Email: "test@example.com"}, nil
}

// ValidateUser valida los datos del usuario
func ValidateUser(u *User) error {
	if u == nil {
		return fmt.Errorf("user is nil")
	}
	if u.Name == "" {
		return fmt.Errorf("name is required")
	}
	return nil
}

// ProcessUser procesa un usuario - LLAMA a GetUser y ValidateUser
func ProcessUser(id int) error {
	user, err := GetUser(id)
	if err != nil {
		return err
	}
	return ValidateUser(user)
}

// unusedFunction - CÓDIGO MUERTO para detectar
func unusedFunction() {
	fmt.Println("This is never called")
}

func main() {
	if err := ProcessUser(1); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("User processed successfully")
}
