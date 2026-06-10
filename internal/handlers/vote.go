package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/Diyarbekoralbaev/emergency_callback_go/internal/db/sqlc"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type voteData struct {
	Callback  voteCallback
	Range5    []int
	ThanksURL string
}

type voteCallback struct {
	VoteUUID string
}

func (s *Server) VotePage(c *gin.Context) {
	ctx := c.Request.Context()
	voteUUID := c.Param("uuid")
	u, err := uuid.Parse(voteUUID)
	if err != nil {
		s.renderStandalone(c, "callbacks/vote_error.html", gin.H{"Error": "Неверная ссылка"})
		return
	}

	cb, err := s.Q.GetCallbackByVoteUUID(ctx, pgUUID(u))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			s.renderStandalone(c, "callbacks/vote_error.html", gin.H{"Error": "Вызов не найден"})
			return
		}
		s.renderStandalone(c, "callbacks/vote_error.html", gin.H{"Error": err.Error()})
		return
	}
	hasRating, _ := s.Q.CallbackHasRating(ctx, cb.ID)
	if hasRating {
		s.renderStandalone(c, "callbacks/vote_already_used.html", gin.H{})
		return
	}
	data := voteData{
		Callback:  voteCallback{VoteUUID: voteUUID},
		Range5:    []int{1, 2, 3, 4, 5},
		ThanksURL: "/vote/" + voteUUID + "/thanks/",
	}
	s.renderStandalone(c, "callbacks/vote.html", data)
}

func (s *Server) SubmitVote(c *gin.Context) {
	ctx := c.Request.Context()
	voteUUID := c.Param("uuid")
	u, err := uuid.Parse(voteUUID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Noto'g'ri silteme"})
		return
	}

	cb, err := s.Q.GetCallbackByVoteUUID(ctx, pgUUID(u))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Vyzov tabilmadi"})
		return
	}

	hasRating, _ := s.Q.CallbackHasRating(ctx, cb.ID)
	if hasRating {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Siz allaqashan bahaladingiz"})
		return
	}

	ratingStr := c.PostForm("rating")
	comment := strings.TrimSpace(c.PostForm("comment"))

	r, err := strconv.Atoi(ratingStr)
	if err != nil || r < 1 || r > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Noto'g'ri baha"})
		return
	}
	if len(comment) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Pikir 500 tańbadan aspawin kerek"})
		return
	}

	var commentPtr *string
	if comment != "" {
		commentPtr = &comment
	}

	if _, err := s.Q.CreateRating(ctx, sqlc.CreateRatingParams{
		CallbackRequestID: cb.ID,
		Rating:            int32(r),
		Comment:           commentPtr,
		PhoneNumber:       cb.PhoneNumber,
		TeamID:            cb.TeamID,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Qátelik júz berdi"})
		return
	}

	if err := s.Q.UpdateCallbackVotedViaSMS(ctx, cb.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Qátelik júz berdi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Rahmet! Siziń bahańız qabıl qılındı"})
}

func (s *Server) VoteThanks(c *gin.Context) {
	s.renderStandalone(c, "callbacks/vote_thanks.html", gin.H{})
}

// pgUUID converts a google/uuid.UUID to a pgtype.UUID.
func pgUUID(u uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: u, Valid: true}
}
