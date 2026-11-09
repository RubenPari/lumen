package naming

// Code generated from Jellyfin naming tests; DO NOT EDIT.

var multiDiscCases = []struct {
	path string
	want bool
}{
	{"", false},
	{"C:/", false},
	{"/home/", false},
	{"blah blah", false},
	{"D:/music/weezer/03 Pinkerton", false},
	{"D:/music/michael jackson/Bad (2012 Remaster)", false},
	{"cd1", true},
	{"disc18", true},
	{"disk10", true},
	{"vol7", true},
	{"volume1", true},
	{"cd 1", true},
	{"disc 1", true},
	{"disk 1", true},
	{"disk", false},
	{"disk ·", false},
	{"disk a", false},
	{"disk volume", false},
	{"disc disc", false},
	{"disk disc 6", false},
	{"cd  - 1", true},
	{"disc- 1", true},
	{"disk - 1", true},
	{"Disc 01 (Hugo Wolf · 24 Lieder)", true},
	{"Disc 04 (Encores and Folk Songs)", true},
	{"Disc04 (Encores and Folk Songs)", true},
	{"Disc 04(Encores and Folk Songs)", true},
	{"Disc04(Encores and Folk Songs)", true},
	{"D:/Video/MBTestLibrary/VideoTest/music/.38 special/anth/Disc 2", true},
	{"[1985] Opportunities (Let's make lots of money) (1985)", false},
	{"Blah 04(Encores and Folk Songs)", false},
}
var bookCases = []struct {
	in, name, series string
	index, year      *int
}{
	{"Sherlock Holmes (1887) #1 (of 4) (1887)", "", "Sherlock Holmes", ip(1), ip(1887)},
	{"Sherlock Holmes #2", "", "Sherlock Holmes", ip(2), nil},
	{"Sherlock Holmes (1887) #1", "", "Sherlock Holmes", ip(1), nil},
	{"Sherlock Holmes #2 (1890)", "", "Sherlock Holmes", ip(2), ip(1890)},
	{"A Study in Scarlet (Sherlock Holmes, #1) (1887)", "A Study in Scarlet", "Sherlock Holmes", ip(1), ip(1887)},
	{"The Adventures of Sherlock Holmes (Sherlock Holmes, #5)", "The Adventures of Sherlock Holmes", "Sherlock Holmes", ip(5), nil},
	{"The Sign of the Four (1890)", "The Sign of the Four", "", nil, ip(1890)},
	{"The Valley of Fear (1915)", "The Valley of Fear", "", nil, ip(1915)},
	{"2 - The Sign of the Four (1890)", "The Sign of the Four", "", ip(2), ip(1890)},
	{"4 - The Valley of Fear", "The Valley of Fear", "", ip(4), nil},
	{"A Study in Scarlet", "A Study in Scarlet", "", nil, nil},
	{"The Adventures of Sherlock Holmes", "The Adventures of Sherlock Holmes", "", nil, nil},
	{"00 - Dracula's Guest (1914)", "Dracula's Guest", "", ip(0), ip(1914)},
	{"01 - Dracula (1897)", "Dracula", "", ip(1), ip(1897)},
	{"2.0 - Twenty Thousand Leagues Under the Sea", "Twenty Thousand Leagues Under the Sea", "", ip(2), nil},
	{"2.1 - The Blockade Runners", "2.1 - The Blockade Runners", "", nil, nil},
}
var comicCases = []struct {
	in, name, series      string
	chapter, volume, year *int
}{
	{"Captain Marvel Adventures v01 (1941)", "Captain Marvel Adventures v01", "", nil, ip(1), ip(1941)},
	{"Captain Marvel Adventures c120", "Captain Marvel Adventures c120", "", ip(120), nil, nil},
	{"Captain Marvel Adventures v01 c120", "Captain Marvel Adventures v01 c120", "", ip(120), ip(1), nil},
}
