package db

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// CountGrouped counts items of itemType grouped by column (parent_id or artist_id).
// Ids with no children are absent from the map.
func (d *DB) CountGrouped(column, itemType string, ids []string) (map[string]int, error) {
	switch column {
	case "parent_id", "artist_id":
	default:
		return nil, fmt.Errorf("count grouped: column %q", column)
	}
	out := map[string]int{}
	for start := 0; start < len(ids); start += 500 {
		end := min(start+500, len(ids))
		chunk := ids[start:end]
		args := make([]any, 0, len(chunk)+1)
		args = append(args, itemType)
		for _, id := range chunk {
			args = append(args, id)
		}
		rows, err := d.Query(`SELECT `+column+`, COUNT(*) FROM items WHERE type=? AND `+column+` IN (`+placeholders(len(chunk))+`) GROUP BY `+column, args...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id string
			var n int
			if err := rows.Scan(&id, &n); err != nil {
				rows.Close()
				return nil, err
			}
			out[id] = n
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return nil, err
		}
		rows.Close()
	}
	return out, nil
}

// MediaTotals is the dashboard aggregate over the library.
type MediaTotals struct {
	Counts   map[string]int
	Size     int64
	Duration float64
}

// MediaTotals returns item counts by type, total file size, and movie/episode runtime in seconds.
func (d *DB) MediaTotals() (MediaTotals, error) {
	out := MediaTotals{Counts: map[string]int{}}
	rows, err := d.Query(`SELECT type, COUNT(*) FROM items WHERE extra_type='' GROUP BY type`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var typ string
		var n int
		if err := rows.Scan(&typ, &n); err != nil {
			return out, err
		}
		out.Counts[typ] = n
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	if err := d.QueryRow(`SELECT COALESCE(SUM(size),0) FROM items WHERE is_folder=0`).Scan(&out.Size); err != nil {
		return out, err
	}
	if err := d.QueryRow(`SELECT COALESCE(SUM(duration),0) FROM items WHERE type IN ('Movie','Episode') AND extra_type=''`).Scan(&out.Duration); err != nil {
		return out, err
	}
	return out, nil
}

// CreateUser inserts an account.
func (d *DB) CreateUser(u *User) error {
	_, err := d.Exec(`INSERT INTO users(id,name,password_hash,is_admin,created_at) VALUES(?,?,?,?,?)`, u.ID, u.Name, u.PasswordHash, u.IsAdmin, u.CreatedAt)
	return err
}

// UpdateUser writes name, password hash, and admin flag.
func (d *DB) UpdateUser(u *User) error {
	_, err := d.Exec(`UPDATE users SET name=?, password_hash=?, is_admin=? WHERE id=?`, u.Name, u.PasswordHash, u.IsAdmin, u.ID)
	return err
}

// SetPassword replaces a user's password hash.
func (d *DB) SetPassword(id, hash string) error {
	_, err := d.Exec(`UPDATE users SET password_hash=? WHERE id=?`, hash, id)
	return err
}

// DeleteUser removes an account. Sessions go with the foreign key.
func (d *DB) DeleteUser(id string) error {
	_, err := d.Exec(`DELETE FROM users WHERE id=?`, id)
	return err
}

// CountAdmins returns how many users have the administrator flag.
func (d *DB) CountAdmins() (int, error) {
	var n int
	err := d.QueryRow(`SELECT COUNT(*) FROM users WHERE is_admin=1`).Scan(&n)
	return n, err
}

// DeleteLibrary removes a library and its items.
func (d *DB) DeleteLibrary(id string) error {
	if _, err := d.Exec(`DELETE FROM items WHERE library_id=?`, id); err != nil {
		return err
	}
	_, err := d.Exec(`DELETE FROM libraries WHERE id=?`, id)
	return err
}

// SetMetadataLocked sets the lock bit without rewriting the rest of the row.
func (d *DB) SetMetadataLocked(id string, locked bool) error {
	_, err := d.Exec(`UPDATE items SET metadata_locked=? WHERE id=?`, locked, id)
	return err
}

// Session is a login token.
type Session struct {
	Token, UserID, Device, Client, DeviceID, Version string
}

// CreateSession inserts a session. Client fields may be empty for the web UI.
func (d *DB) CreateSession(s Session) error {
	now := Now()
	_, err := d.Exec(`INSERT INTO sessions(token,user_id,device,created_at,last_seen,client,device_id,app_version) VALUES(?,?,?,?,?,?,?,?)`,
		s.Token, s.UserID, s.Device, now, now, s.Client, s.DeviceID, s.Version)
	return err
}

// LookupSession returns the user id and last-seen time for a token.
func (d *DB) LookupSession(token string) (userID string, lastSeen int64, err error) {
	err = d.QueryRow(`SELECT user_id,last_seen FROM sessions WHERE token=?`, token).Scan(&userID, &lastSeen)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrNotFound
	}
	return userID, lastSeen, err
}

// TouchSession refreshes last_seen.
func (d *DB) TouchSession(token string) error {
	_, err := d.Exec(`UPDATE sessions SET last_seen=? WHERE token=?`, Now(), token)
	return err
}

// DeleteSession removes a token. A missing token is not an error.
func (d *DB) DeleteSession(token string) error {
	_, err := d.Exec(`DELETE FROM sessions WHERE token=?`, token)
	return err
}

// DisplayPref returns stored Jellyfin display preferences.
func (d *DB) DisplayPref(userID, prefID, client string) (string, error) {
	var data string
	err := d.QueryRow(`SELECT data FROM display_prefs WHERE user_id=? AND pref_id=? AND client=?`, userID, prefID, client).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return data, err
}

// SetDisplayPref upserts Jellyfin display preferences.
func (d *DB) SetDisplayPref(userID, prefID, client, data string) error {
	_, err := d.Exec(`INSERT INTO display_prefs(user_id,pref_id,client,data) VALUES(?,?,?,?) ON CONFLICT(user_id,pref_id,client) DO UPDATE SET data=excluded.data`, userID, prefID, client, data)
	return err
}

// CountLiveChannels returns the number of guide channels.
func (d *DB) CountLiveChannels() (int, error) {
	var n int
	err := d.QueryRow(`SELECT COUNT(*) FROM livetv_channels`).Scan(&n)
	return n, err
}
