package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// album represents data about a record album.
type Album struct {
	ID     string  `json:"id"`
	Title  string  `json:"title"`
	Artist string  `json:"artist"`
	Price  float64 `json:"price"`
}

/*
This slice acts like a fake in memory database for learning purposes.
In real apps, you would replace this with a DB.
*/
var albums = []Album{
    {ID: "1", Title: "Blue Train", Artist: "John Coltrane", Price: 56.99},
    {ID: "2", Title: "Jeru", Artist: "Gerry Mulligan", Price: 17.99},
    {ID: "3", Title: "Sarah Vaughan and Clifford Brown", Artist: "Sarah Vaughan", Price: 39.99},
}

func main() {
	// gin.Default gives you a router
	router := gin.Default()
	// Register endpoints
	router.GET("/albums", getAlbums)        // Read all albums
	router.GET("/albums/:id", getAlbumByID) // Read one album by ID
	router.POST("/albums", postAlbums)      // Create a new album

	router.Run("0.0.0.0:8080")

}

/*
GET /albums
Returns all albums as JSON.
*/
func getAlbums(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, albums)
}

/*
POST /albums
adds an album from JSON received in the request body.
*/
func postAlbums(c *gin.Context) {
	var newAlbum Album
	// BindJSON parses the request JSON into newAlbum
	// If JSON is invalid, return 400 Bad Request
	if err := c.BindJSON(&newAlbum); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	// Basic validation
	if newAlbum.ID == "" || newAlbum.Title == "" || newAlbum.Artist == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "id, title, and artist are required"})
		return
	}
	// Simple duplicate ID check
	if indexOfAlbumByID(newAlbum.ID) != -1 {
		c.JSON(http.StatusConflict, gin.H{"message": "album with this id already exists"})
		return
	}
	 // Add the new album to the slice.
	albums = append(albums, newAlbum)
	c.IndentedJSON(http.StatusCreated, newAlbum)
}

/*
GET /albums/:id
Reads path parameter :id, searches for an album, returns it if found.
*/
func getAlbumByID(c *gin.Context) {
	id := c.Param("id")
	index := indexOfAlbumByID(id)
	if index == -1 {
		c.JSON(http.StatusNotFound, gin.H{"message": "album not found"})
		return
	}
	c.IndentedJSON(http.StatusOK, albums[index])
}

/*
Helper method to find an album index by ID.
Returns -1 if not found.
*/
func indexOfAlbumByID(id string) int {
	for i, a := range albums {
		if a.ID == id {
			return i
		}
	}
	return -1
}
