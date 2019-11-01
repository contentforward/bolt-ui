package auth

import (
	"encoding/json"
	"time"

	"github.com/boreq/eggplant/errors"
	"github.com/boreq/eggplant/logging"
	"github.com/boreq/eggplant/pkg/service/application/auth"
	bolt "go.etcd.io/bbolt"
)

type PasswordHash []byte

type PasswordHasher interface {
	Hash(password string) (PasswordHash, error)
	Compare(hashedPassword PasswordHash, password string) error
}

type AccessTokenGenerator interface {
	Generate(username string) (auth.AccessToken, error)
	GetUsername(token auth.AccessToken) (string, error)
}

type user struct {
	Username string       `json:"username"`
	Password PasswordHash `json:"password"`
	Sessions []session
}

type session struct {
	Token    auth.AccessToken
	LastSeen time.Time
}

type UserRepository struct {
	db                   *bolt.DB
	passwordHasher       PasswordHasher
	accessTokenGenerator AccessTokenGenerator
	bucket               []byte
	log                  logging.Logger
}

func NewUserRepository(
	db *bolt.DB,
	passwordHasher PasswordHasher,
	accessTokenGenerator AccessTokenGenerator,
) (*UserRepository, error) {
	bucket := []byte("users")

	if err := db.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
			return errors.Wrap(err, "could not create a bucket")
		}
		return nil
	}); err != nil {
		return nil, errors.Wrap(err, "update failed")
	}

	return &UserRepository{
		passwordHasher:       passwordHasher,
		accessTokenGenerator: accessTokenGenerator,
		db:                   db,
		bucket:               bucket,
		log:                  logging.New("userRepository"),
	}, nil
}

func (r *UserRepository) RegisterInitial(username, password string) error {
	if err := r.validate(username, password); err != nil {
		return errors.Wrap(err, "invalid parameters")
	}

	passwordHash, err := r.passwordHasher.Hash(password)
	if err != nil {
		return errors.Wrap(err, "hashing the password failed")
	}

	u := user{
		Username: username,
		Password: passwordHash,
	}

	j, err := json.Marshal(u)
	if err != nil {
		return errors.Wrap(err, "marshaling to json failed")
	}

	return r.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(r.bucket)
		if !bucketIsEmpty(b) {
			return errors.New("there are existing users")
		}
		return b.Put([]byte(u.Username), j)
	})
}

func (r *UserRepository) Login(username, password string) (auth.AccessToken, error) {
	if err := r.validate(username, password); err != nil {
		return "", errors.Wrap(err, "invalid parameters")
	}

	var token auth.AccessToken

	if err := r.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(r.bucket)
		j := b.Get([]byte(username))
		if j == nil {
			return errors.New("user does not exist")
		}

		var u user
		if err := json.Unmarshal(j, &u); err != nil {
			return errors.Wrap(err, "json unmarshal failed")
		}

		if err := r.passwordHasher.Compare(u.Password, password); err != nil {
			return errors.Wrap(err, "invalid credentials")
		}

		t, err := r.accessTokenGenerator.Generate(username)
		if err != nil {
			return errors.Wrap(err, "could not create an access token")
		}
		token = t

		s := session{
			Token: t,
		}

		u.Sessions = append(u.Sessions, s)

		j, err = json.Marshal(u)
		if err != nil {
			return errors.Wrap(err, "marshaling to json failed")
		}

		return b.Put([]byte(username), j)
	}); err != nil {
		return "", errors.Wrap(err, "transaction failed")
	}

	return token, nil

}

func (r *UserRepository) CheckAccessToken(token auth.AccessToken) (auth.User, error) {
	username, err := r.accessTokenGenerator.GetUsername(token)
	if err != nil {
		r.log.Warn("could not get the username", "err", err)
		return auth.User{}, auth.ErrUnauthorized
	}

	var foundUser user
	if err := r.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(r.bucket)

		u, err := r.getUser(b, username)
		if err != nil {
			return errors.Wrap(err, "could not get the user")
		}

		if u == nil {
			r.log.Warn("user does't exist", "username", username)
			return auth.ErrUnauthorized
		}

		for i := range u.Sessions {
			if u.Sessions[i].Token == token {
				u.Sessions[i].LastSeen = time.Now()
				foundUser = *u
				return r.putUser(b, *u)
			}
		}

		return errors.New("invalid token")
	}); err != nil {
		return auth.User{}, errors.Wrap(err, "transaction failed")
	}

	u := auth.User{
		Username: foundUser.Username,
	}

	return u, nil
}

func (r *UserRepository) Logout(token auth.AccessToken) error {
	return errors.New("not implemented")
}

func (r *UserRepository) validate(username, password string) error {
	if username == "" {
		return errors.New("username can't be empty")
	}

	if password == "" {
		return errors.New("password can't be empty")
	}

	return nil
}

func (r *UserRepository) Count() (int, error) {
	var count int
	if err := r.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(r.bucket)
		count = b.Stats().KeyN
		return nil
	}); err != nil {
		return 0, errors.Wrap(err, "view error")
	}
	return count, nil
}

func (r *UserRepository) getUser(b *bolt.Bucket, username string) (*user, error) {
	j := b.Get([]byte(username))
	if j == nil {
		return nil, nil
	}

	u := &user{}
	if err := json.Unmarshal(j, u); err != nil {
		return nil, errors.Wrap(err, "json unmarshal failed")
	}

	return u, nil
}

func (r *UserRepository) putUser(b *bolt.Bucket, u user) error {
	j, err := json.Marshal(u)
	if err != nil {
		return errors.Wrap(err, "marshaling to json failed")
	}

	return b.Put([]byte(u.Username), j)
}

func bucketIsEmpty(b *bolt.Bucket) bool {
	key, value := b.Cursor().First()
	return key == nil && value == nil
}
