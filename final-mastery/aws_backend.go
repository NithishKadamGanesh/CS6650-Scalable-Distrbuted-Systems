package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type albumItem struct {
	AlbumID     string `dynamodbav:"album_id"`
	Title       string `dynamodbav:"title"`
	Description string `dynamodbav:"description"`
	Owner       string `dynamodbav:"owner"`
	NextSeq     int64  `dynamodbav:"next_seq"`
}

type photoItem struct {
	PhotoID    string `dynamodbav:"photo_id"`
	AlbumID    string `dynamodbav:"album_id"`
	Seq        int64  `dynamodbav:"seq"`
	Status     string `dynamodbav:"status"`
	URL        string `dynamodbav:"url,omitempty"`
	ObjectKey  string `dynamodbav:"object_key"`
	TempPath   string `dynamodbav:"temp_path,omitempty"`
	UploadedAt string `dynamodbav:"uploaded_at"`
}

func (a *App) upsertAlbumAWS(ctx context.Context, album Album) (int, Album, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	input := &dynamodb.UpdateItemInput{
		TableName: aws.String(a.cfg.AlbumsTable),
		Key: map[string]types.AttributeValue{
			"album_id": &types.AttributeValueMemberS{Value: album.AlbumID},
		},
		UpdateExpression: aws.String("SET #title = :title, #description = :description, #owner = :owner, #updated_at = :updated_at, #next_seq = if_not_exists(#next_seq, :zero)"),
		ExpressionAttributeNames: map[string]string{
			"#title":       "title",
			"#description": "description",
			"#owner":       "owner",
			"#updated_at":  "updated_at",
			"#next_seq":    "next_seq",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":title":       &types.AttributeValueMemberS{Value: album.Title},
			":description": &types.AttributeValueMemberS{Value: album.Description},
			":owner":       &types.AttributeValueMemberS{Value: album.Owner},
			":updated_at":  &types.AttributeValueMemberS{Value: now},
			":zero":        &types.AttributeValueMemberN{Value: "0"},
		},
		ReturnValues: types.ReturnValueAllOld,
	}
	out, err := a.ddb.UpdateItem(ctx, input)
	if err != nil {
		return 0, Album{}, err
	}
	if len(out.Attributes) == 0 {
		return http.StatusCreated, album, nil
	}
	return http.StatusOK, album, nil
}

func (a *App) getAlbumAWS(ctx context.Context, albumID string) (Album, error) {
	out, err := a.ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(a.cfg.AlbumsTable),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"album_id": &types.AttributeValueMemberS{Value: albumID},
		},
	})
	if err != nil {
		return Album{}, err
	}
	if len(out.Item) == 0 {
		return Album{}, sql.ErrNoRows
	}
	var item albumItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return Album{}, err
	}
	return Album{
		AlbumID:     item.AlbumID,
		Title:       item.Title,
		Description: item.Description,
		Owner:       item.Owner,
	}, nil
}

func (a *App) listAlbumsAWS(ctx context.Context) ([]Album, error) {
	items := make([]Album, 0)
	var startKey map[string]types.AttributeValue
	for {
		out, err := a.ddb.Scan(ctx, &dynamodb.ScanInput{
			TableName:         aws.String(a.cfg.AlbumsTable),
			ExclusiveStartKey: startKey,
		})
		if err != nil {
			return nil, err
		}
		for _, raw := range out.Items {
			var item albumItem
			if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
				return nil, err
			}
			items = append(items, Album{
				AlbumID:     item.AlbumID,
				Title:       item.Title,
				Description: item.Description,
				Owner:       item.Owner,
			})
		}
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		startKey = out.LastEvaluatedKey
	}
	sort.Slice(items, func(i, j int) bool { return items[i].AlbumID < items[j].AlbumID })
	return items, nil
}

func (a *App) createPhotoAWS(ctx context.Context, albumID string, file io.Reader, filename string) (PhotoAccepted, error) {
	photoID := newUUID()
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".bin"
	}
	objectKey := fmt.Sprintf("photos/%s%s", photoID, ext)

	tempFile, _, err := writeReaderToTempFile(file, "album-store-upload-*"+ext)
	if err != nil {
		return PhotoAccepted{}, err
	}

	seqOut, err := a.ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(a.cfg.AlbumsTable),
		Key: map[string]types.AttributeValue{
			"album_id": &types.AttributeValueMemberS{Value: albumID},
		},
		UpdateExpression: aws.String("SET #next_seq = if_not_exists(#next_seq, :zero) + :one"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":zero": &types.AttributeValueMemberN{Value: "0"},
			":one":  &types.AttributeValueMemberN{Value: "1"},
		},
		ConditionExpression: aws.String("attribute_exists(#album_id)"),
		ExpressionAttributeNames: map[string]string{
			"#album_id": "album_id",
			"#next_seq": "next_seq",
		},
		ReturnValues:        types.ReturnValueUpdatedNew,
	})
	if err != nil {
		tempFile.Close()
		_ = os.Remove(tempFile.Name())
		var cond *types.ConditionalCheckFailedException
		if errors.As(err, &cond) {
			return PhotoAccepted{}, sql.ErrNoRows
		}
		return PhotoAccepted{}, err
	}

	var seqValue int64
	if attr, ok := seqOut.Attributes["next_seq"].(*types.AttributeValueMemberN); ok {
		fmt.Sscanf(attr.Value, "%d", &seqValue)
	}

	item := photoItem{
		PhotoID:    photoID,
		AlbumID:    albumID,
		Seq:        seqValue,
		Status:     "processing",
		ObjectKey:  objectKey,
		TempPath:   tempFile.Name(),
		UploadedAt: time.Now().UTC().Format(time.RFC3339),
	}
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return PhotoAccepted{}, err
	}
	if _, err := a.ddb.PutItem(ctx, &dynamodb.PutItemInput{
		TableName: aws.String(a.cfg.PhotosTable),
		Item:      av,
	}); err != nil {
		tempFile.Close()
		_ = os.Remove(tempFile.Name())
		return PhotoAccepted{}, err
	}
	tempFile.Close()

	a.enqueuePhoto(photoID)
	return PhotoAccepted{PhotoID: photoID, Seq: seqValue, Status: "processing"}, nil
}

func (a *App) processPhotoAWS(photoID string) {
	ctx := context.Background()
	item, err := a.getPhotoItemAWS(ctx, photoID)
	if err != nil || item.Status != "processing" {
		return
	}
	if a.cfg.ProcessingDelay > 0 {
		time.Sleep(a.cfg.ProcessingDelay)
	}

	if item.TempPath == "" {
		a.markPhotoFailedAWS(ctx, photoID, errors.New("missing temp file path"))
		return
	}

	file, err := os.Open(item.TempPath)
	if err != nil {
		a.markPhotoFailedAWS(ctx, photoID, err)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		a.markPhotoFailedAWS(ctx, photoID, err)
		return
	}

	_, err = a.uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(a.cfg.S3Bucket),
		Key:           aws.String(item.ObjectKey),
		Body:          file,
		ContentLength: aws.Int64(info.Size()),
	})
	if err != nil {
		a.markPhotoFailedAWS(ctx, photoID, err)
		return
	}

	_, err = a.ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(a.cfg.PhotosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
		UpdateExpression:    aws.String("SET #status = :completed, #url = :url REMOVE temp_path"),
		ConditionExpression: aws.String("#status = :processing"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
			"#url":    "url",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":processing": &types.AttributeValueMemberS{Value: "processing"},
			":completed":  &types.AttributeValueMemberS{Value: "completed"},
			":url":        &types.AttributeValueMemberS{Value: fmt.Sprintf("%s/media/%s", a.cfg.PublicBaseURL, photoID)},
		},
	})
	if err != nil {
		var cond *types.ConditionalCheckFailedException
		if errors.As(err, &cond) {
			_, _ = a.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(a.cfg.S3Bucket),
				Key:    aws.String(item.ObjectKey),
			})
			_ = os.Remove(item.TempPath)
			return
		}
	}
	_ = os.Remove(item.TempPath)
}

func (a *App) markPhotoFailedAWS(ctx context.Context, photoID string, processErr error) {
	_, _ = a.ddb.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName: aws.String(a.cfg.PhotosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
		UpdateExpression: aws.String("SET #status = :failed, error_message = :error REMOVE temp_path"),
		ExpressionAttributeNames: map[string]string{
			"#status": "status",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":failed": &types.AttributeValueMemberS{Value: "failed"},
			":error":  &types.AttributeValueMemberS{Value: processErr.Error()},
		},
	})
}

func (a *App) getPhotoAWS(ctx context.Context, albumID, photoID string) (PhotoStatus, error) {
	item, err := a.getPhotoItemAWS(ctx, photoID)
	if err != nil {
		return PhotoStatus{}, err
	}
	if item.AlbumID != albumID {
		return PhotoStatus{}, sql.ErrNoRows
	}
	status := PhotoStatus{
		PhotoID: item.PhotoID,
		AlbumID: item.AlbumID,
		Seq:     item.Seq,
		Status:  item.Status,
	}
	if item.Status == "completed" {
		status.URL = fmt.Sprintf("%s/media/%s", a.cfg.PublicBaseURL, item.PhotoID)
	}
	return status, nil
}

func (a *App) deletePhotoAWS(ctx context.Context, albumID, photoID string) error {
	item, err := a.getPhotoItemAWS(ctx, photoID)
	if err != nil {
		if isNoRows(err) {
			return nil
		}
		return err
	}
	if item.AlbumID != albumID {
		return nil
	}
	if item.TempPath != "" {
		_ = os.Remove(item.TempPath)
	}
	_, err = a.ddb.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName: aws.String(a.cfg.PhotosTable),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
	})
	if err != nil {
		return err
	}
	_, _ = a.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(a.cfg.S3Bucket),
		Key:    aws.String(item.ObjectKey),
	})
	return nil
}

func (a *App) handleMediaAWS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	photoID := strings.TrimPrefix(r.URL.Path, "/media/")
	item, err := a.getPhotoItemAWS(r.Context(), photoID)
	if err != nil || item.Status != "completed" {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	out, err := a.s3.GetObject(r.Context(), &s3.GetObjectInput{
		Bucket: aws.String(a.cfg.S3Bucket),
		Key:    aws.String(item.ObjectKey),
	})
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "not found")
		return
	}
	defer out.Body.Close()
	if out.ContentType != nil {
		w.Header().Set("Content-Type", *out.ContentType)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, out.Body)
}

func (a *App) requeueProcessingAWS() error {
	ctx := context.Background()
	var startKey map[string]types.AttributeValue
	for {
		out, err := a.ddb.Scan(ctx, &dynamodb.ScanInput{
			TableName:         aws.String(a.cfg.PhotosTable),
			ExclusiveStartKey: startKey,
			FilterExpression:  aws.String("#status = :processing"),
			ExpressionAttributeNames: map[string]string{
				"#status": "status",
			},
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":processing": &types.AttributeValueMemberS{Value: "processing"},
			},
		})
		if err != nil {
			return err
		}
		for _, raw := range out.Items {
			var item photoItem
			if err := attributevalue.UnmarshalMap(raw, &item); err != nil {
				return err
			}
			a.enqueuePhoto(item.PhotoID)
		}
		if len(out.LastEvaluatedKey) == 0 {
			break
		}
		startKey = out.LastEvaluatedKey
	}
	return nil
}

func (a *App) getPhotoItemAWS(ctx context.Context, photoID string) (photoItem, error) {
	out, err := a.ddb.GetItem(ctx, &dynamodb.GetItemInput{
		TableName:      aws.String(a.cfg.PhotosTable),
		ConsistentRead: aws.Bool(true),
		Key: map[string]types.AttributeValue{
			"photo_id": &types.AttributeValueMemberS{Value: photoID},
		},
	})
	if err != nil {
		return photoItem{}, err
	}
	if len(out.Item) == 0 {
		return photoItem{}, sql.ErrNoRows
	}
	var item photoItem
	if err := attributevalue.UnmarshalMap(out.Item, &item); err != nil {
		return photoItem{}, err
	}
	return item, nil
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func writeReaderToTempFile(src io.Reader, pattern string) (*os.File, int64, error) {
	tmp, err := os.CreateTemp("", pattern)
	if err != nil {
		return nil, 0, err
	}
	size, copyErr := io.Copy(tmp, src)
	if copyErr != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, 0, copyErr
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return nil, 0, err
	}
	return tmp, size, nil
}
