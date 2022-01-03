package monday

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/machinebox/graphql"
)

type User struct {
	Id    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}
type Board struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}
type Group struct {
	Id    string `json:"id"`
	Title string `json:"title"`
}
type Column struct {
	Id       string `json:"id"`
	Title    string `json:"title"`
	Type     string `json:"type"`         // text, boolean, color, ...
	Settings string `json:"settings_str"` // used to get label index values for color(status) and dropdown column types
}
type ColumnMap map[string]Column // key is column id, provides easy access to a board's column info using column id

type ColumnValue struct {
	Id    string `json:"id"`    // column id
	Value string `json:"value"` // see func DecodeValue below
}

type Item struct {
	Id           string
	GroupId      string
	Name         string
	ColumnValues []ColumnValue
}

// following types used to convert value from/to specific Monday value type
type DateTime struct {
	Date string `json:"date"`
	Time string `json:"time"`
}
type StatusIndex struct {
	Index int `json:"index"`
}
type PersonTeam struct {
	Id   int    `json:"id"`
	Kind string `json:"kind"` // "person" or "team"
}
type People struct {
	PersonsAndTeams []PersonTeam `json:"personsAndTeams"`
}
type Checkbox struct {
	Checked string `json:"checked"`
}

const Endpoint = "https://api.monday.com/v2/"

type Client struct {
	token  string
	client *graphql.Client
}

// NewClient returns a authenticated client for the Monday.com API
func NewClient(authToken string) *Client {
	return &Client{token: authToken, client: graphql.NewClient(Endpoint)}
}

// RunRequest executes request and decodes response into response parm (address of object)
func (c *Client) runRequest(req *graphql.Request, response interface{}) error {
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Authorization", c.token)
	req.Header.Set("Content-Type", "application/json")
	ctx := context.Background()
	err := c.client.Run(ctx, req, response)
	return err
}

// GetUsers returns []User for all users.
func (c *Client) GetUsers() ([]User, error) {
	req := graphql.NewRequest(`
	    query {
            users {
                id name email
            }
        }
    `)
	var response struct {
		Users []User `json:"users"`
	}
	err := c.runRequest(req, &response)
	return response.Users, err
}

// GetBoards returns []Board for all boards.
func (c *Client) GetBoards() ([]Board, error) {
	req := graphql.NewRequest(`
	    query {
            boards {
                id name
            }
        }
    `)
	var response struct {
		Boards []Board `json:"boards"`
	}
	err := c.runRequest(req, &response)
	return response.Boards, err
}

// GetGroups returns []Group for specified board.
func (c *Client) GetGroups(boardId int) ([]Group, error) {
	req := graphql.NewRequest(`
		query ($boardId: [Int]) {
			boards (ids: $boardId) {
				groups {
					id title
				}	
            }
        }
	`)
	req.Var("boardId", []int{boardId})
	type board struct {
		Groups []Group `json:"groups"`
	}
	var response struct {
		Boards []board `json:"boards"`
	}
	err := c.runRequest(req, &response)
	return response.Boards[0].Groups, err
}

// GetColumns returns []Column for specified boardId.
func (c *Client) GetColumns(boardId int) ([]Column, error) {
	req := graphql.NewRequest(`
	    query ($boardId: [Int]) {
            boards (ids: $boardId) {
                columns {id title type settings_str}
            }
        }
    `)
	req.Var("boardId", []int{boardId})
	type board struct {
		Columns []Column `json:"columns"`
	}
	var response struct {
		Boards []board `json:"boards"`
	}
	err := c.runRequest(req, &response)
	return response.Boards[0].Columns, err
}

// CreateColumnMap returns map[string]Column for specified boardId. Key is columnId.
func (c *Client) CreateColumnMap(boardId int) (ColumnMap, error) {
	var columns []Column
	columnMap := make(ColumnMap)

	columns, err := c.GetColumns(boardId)
	if err != nil {
		return columnMap, err
	}
	for _, column := range columns {
		columnMap[column.Id] = column
	}
	return columnMap, nil
}

// Example of creating columnValues for AddItem
// map entry key is column id; run GetColumns to get column id's
/*
	columnValues := map[string]interface{}{
		"text":   "have a nice day",
		"date":   monday.BuildDate("2019-05-22"),
		"status": monday.BuildStatusIndex(2),
		"people": monday.BuildPeople(123456, 987654),   // parameters are user ids
	}
*/

func BuildDate(date string) DateTime {
	return DateTime{Date: date}
}
func BuildDateTime(date, time string) DateTime {
	return DateTime{Date: date, Time: time}
}
func BuildStatusIndex(index int) StatusIndex {
	return StatusIndex{index}
}
func BuildCheckbox(checked string) Checkbox {
	return Checkbox{checked}
}
func BuildPeople(userIds ...int) People {
	response := People{}
	response.PersonsAndTeams = make([]PersonTeam, len(userIds))
	for i, id := range userIds {
		response.PersonsAndTeams[i] = PersonTeam{id, "person"}
	}
	return response
}

// AddItem adds 1 item to specified board/group. The id of the added item is returned.
func (c *Client) AddItem(boardId int, groupId string, itemName string, columnValues map[string]interface{}) (string, error) {
	req := graphql.NewRequest(`
        mutation ($boardId: Int!, $groupId: String!, $itemName: String!, $colValues: JSON!) {
            create_item (board_id: $boardId, group_id: $groupId, item_name: $itemName, column_values: $colValues ) {
                id
            }
        }
    `)
	jsonValues, _ := json.Marshal(&columnValues)
	log.Println(string(jsonValues))

	req.Var("boardId", boardId)
	req.Var("groupId", groupId)
	req.Var("itemName", itemName)
	req.Var("colValues", string(jsonValues))

	type ItemId struct {
		Id string `json:"id"` // Note value is numeric and not enclosed in quotes, but does not work with type int
	}
	var response struct {
		CreateItem ItemId `json:"create_item"`
	}
	err := c.runRequest(req, &response)
	return response.CreateItem.Id, err
}

// AddItemUpdate adds an update entry to specified item.
func (c *Client) AddItemUpdate(itemId string, msg string) error {
	intItemId, err := strconv.Atoi(itemId)
	if err != nil {
		log.Println("AddItemUpdate - bad itemId", err)
		return err
	}
	req := graphql.NewRequest(`
		mutation ($itemId: Int!, $body: String!) {
			create_update (item_id: $itemId, body: $body ) {
				id
			}
		}
	`)
	req.Var("itemId", intItemId)
	req.Var("body", msg)

	type UpdateId struct {
		Id string `json:"id"`
	}
	var response struct {
		CreateUpdate UpdateId `json:"create_update"`
	}
	err = c.runRequest(req, &response)
	return err
}

// GetItems returns []Item for all items in specified board.
func (c *Client) GetItems(boardId int) ([]Item, error) {
	req := graphql.NewRequest(`	
		query ($boardId: [Int]) {
			boards (ids: $boardId){
				# items (limit: 10) {
				items () {
					id
					group {	id }
					name
					# column_values (ids: ["text", "status", "check"]) {  -- to get specific columns  
					column_values { 
						id value
					}
				}	
			}
		}	
	`)
	req.Var("boardId", []int{boardId})

	type group struct {
		Id string `json:"id"`
	}
	type itemData struct {
		Id           string        `json:"id"`
		Group        group         `json:"group"`
		Name         string        `json:"name"`
		ColumnValues []ColumnValue `json:"column_values"`
	}
	type boardItems struct {
		Items []itemData `json:"items"`
	}
	var response struct {
		Boards []boardItems `json:"boards"`
	}
	items := make([]Item, 0, 1000)
	err := c.runRequest(req, &response)
	if err != nil {
		fmt.Println("GetItems Failed -", err)
		return items, err
	}
	var responseItems []itemData = response.Boards[0].Items
	for _, responseItem := range responseItems {
		items = append(items, Item{
			Id:           responseItem.Id,
			GroupId:      responseItem.Group.Id,
			Name:         responseItem.Name,
			ColumnValues: responseItem.ColumnValues,
		})
	}
	return items, nil
}

// DecodeValues converts column value returned from Monday to a string value
// 	color(status) returns index of label chosen, ex. "3"
// 	boolean(checkbox) returns "true" or "false"
// 	date returns "2019-05-22"
// Types "multi-person" and "dropdown" may have multiple values.
//		for these, a slice of strings is returned
// Use CreateColumnMap to create the columnMap (contains info for all columns in board)
func DecodeValue(columnMap ColumnMap, columnValue ColumnValue) (result1 string, result2 []string, err error) {
	if columnValue.Value == "" {
		return
	}
	column, found := columnMap[columnValue.Id]
	if !found {
		err = errors.New("invalid column id - " + columnValue.Id)
		return
	}
	inVal := []byte(columnValue.Value) // convert input value (string) to []byte, required by json.Unmarshal
	switch column.Type {
	case "text":
		result1 = columnValue.Value
	case "color": // status, return index of value
		var val StatusIndex
		err = json.Unmarshal(inVal, &val)
		result1 = strconv.Itoa(val.Index)
	case "boolean": // checkbox, return true or false
		var val Checkbox
		err = json.Unmarshal(inVal, &val)
		result1 = val.Checked
	case "date":
		var val DateTime
		err = json.Unmarshal(inVal, &val)
		result1 = val.Date
	case "multiple-person":
		result2 = DecodePeople(columnValue.Value)
	case "dropdown":
		result2 = DecodeDropDown(columnValue.Value)
	default:
		err = errors.New("value type not handled - " + column.Type)
	}
	return
}

// DecodePeople returns user id of each person assigned. Use GetUsers to get all user id values.
func DecodePeople(valueIn string) []string {
	var val People
	err := json.Unmarshal([]byte(valueIn), &val)
	if err != nil {
		log.Println("DecodePeople Unmarshal Failed, ", err)
		return nil
	}
	result := make([]string, len(val.PersonsAndTeams))
	for i, person := range val.PersonsAndTeams {
		result[i] = strconv.Itoa(person.Id)
	}
	return result
}

// DecodeDropDown returns ids of value selections. Use DecodeLabels to list Index value for each dropdown label.
func DecodeDropDown(valueIn string) []string {
	var val struct {
		Ids []int `json:"ids"`
	}
	err := json.Unmarshal([]byte(valueIn), &val)
	if err != nil {
		log.Println("DecodeDropDown Unmarshal Failed, ", err)
		return nil
	}
	result := make([]string, len(val.Ids))
	for i, id := range val.Ids {
		result[i] = strconv.Itoa(id)
	}
	return result
}

// DecodeLabels displays index value of all labels for a column. Uses column settings_str (see GetColumns).
// Use for Status(color) and Dropdown fields.
func DecodeLabels(settings_str, columnType string) {
	var statusLabels struct {
		Labels         map[string]string `json:"labels"`             // index: label
		LabelPositions map[string]int    `json:"label_positions_v2"` // index: position
	}
	type dropdownEntry struct {
		Id   int    `json:"id"`
		Name string `json:"name"`
	}
	var dropdownLabels struct {
		Labels []dropdownEntry `json:"labels"`
	}

	if columnType == "color" {
		err := json.Unmarshal([]byte(settings_str), &statusLabels)
		if err != nil {
			log.Fatal("DecodeLabels Failed", err)
		}
		for index, label := range statusLabels.Labels {
			fmt.Println(index, label)
		}
	}
	if columnType == "dropdown" {
		err := json.Unmarshal([]byte(settings_str), &dropdownLabels)
		if err != nil {
			log.Fatal("DecodeLabels Failed", err)
		}
		for _, label := range dropdownLabels.Labels {
			fmt.Println(label.Id, label.Name)
		}
	}
}
