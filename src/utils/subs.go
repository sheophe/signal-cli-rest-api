package utils

import (
	"fmt"

	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
)

type AlreadyLinkedError struct {
	Number   string
	ThisUser bool
}

func (e AlreadyLinkedError) Error() string {
	var to string
	if e.ThisUser {
		to = "to this user"
	} else {
		to = "to another user"
	}
	return fmt.Sprintf("number %s is already linked %s", e.Number, to)
}

func NewAlreadyLinkedError(number string, thisUser bool) AlreadyLinkedError {
	return AlreadyLinkedError{Number: number, ThisUser: thisUser}
}

type LinkedNumber struct {
	Number    string `gorm:"primaryKey"`
	Sub       string `gorm:"not null"`
	ServiceID int64  `gorm:"not null"`
}

type SubStorage struct {
	*gorm.DB
}

func NewSubStorage(dbFile string) (*SubStorage, error) {
	db, err := gorm.Open("sqlite3", dbFile)
	if err != nil {
		return nil, err
	}
	db = db.AutoMigrate(&LinkedNumber{})
	return &SubStorage{db}, nil
}

func (s *SubStorage) GetSubByNumber(number string) (string, bool) {
	row := LinkedNumber{}
	err := s.DB.Model(&row).Where("number = ?", number).First(&row).Error
	if err != nil {
		return "", false
	}
	return row.Sub, true
}

func (s *SubStorage) CheckIfSubIsValid(sub, number string) error {
	foundSub, ok := s.GetSubByNumber(number)
	if !ok {
		return fmt.Errorf("number %s not found", number)
	}
	if sub != foundSub {
		return fmt.Errorf("number %s is linked to another user", number)
	}
	return nil
}

func (s *SubStorage) CheckIfAlreadyLinked(sub, number string) error {
	foundSub, ok := s.GetSubByNumber(number)
	if !ok {
		return nil
	}
	return NewAlreadyLinkedError(number, sub == foundSub)
}

func (s *SubStorage) LinkSub(sub, number string, serviceID int64) error {
	foundSub, ok := s.GetSubByNumber(number)
	if ok {
		return NewAlreadyLinkedError(number, sub == foundSub)
	}
	row := LinkedNumber{
		Sub:       sub,
		Number:    number,
		ServiceID: serviceID,
	}
	return s.Create(row).Error
}
