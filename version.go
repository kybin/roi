package roi

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// CreateTableIfNotExistShowsStmt는 DB에 versions 테이블을 생성하는 sql 구문이다.
// 테이블은 타입보다 많은 정보를 담고 있을수도 있다.
var CreateTableIfNotExistsVersionsStmt = `CREATE TABLE IF NOT EXISTS versions (
	show STRING NOT NULL CHECK (length(show) > 0) CHECK (show NOT LIKE '% %'),
	grp STRING NOT NULL CHECK (length(grp) > 0) CHECK (grp NOT LIKE '% %'),
	unit STRING NOT NULL CHECK (length(unit) > 0) CHECK (unit NOT LIKE '% %'),
	task STRING NOT NULL CHECK (length(task) > 0) CHECK (task NOT LIKE '% %'),
	version STRING NOT NULL CHECK (length(version) > 0),
	owner STRING NOT NULL CHECK (length(owner) > 0),
	output_files STRING[] NOT NULL,
	images STRING[] NOT NULL,
	mov STRING NOT NULL,
	work_file STRING NOT NULL,
	UNIQUE(show, unit, task, version),
	CONSTRAINT versions_pk PRIMARY KEY (show, grp, unit, task, version)
)`

// Version은 특정 태스크의 하나의 버전이다.
type Version struct {
	Show    string `db:"show"`
	Group   string `db:"grp"` // group이 sql 구문이기 때문에 줄여서 씀.
	Unit    string `db:"unit"`
	Task    string `db:"task"`
	Version string `db:"version"` // 버전명

	Owner       string   `db:"owner"`        // 버전 소유자
	OutputFiles []string `db:"output_files"` // 결과물 경로
	Images      []string `db:"images"`       // 결과물을 확인할 수 있는 이미지
	Mov         string   `db:"mov"`          // 결과물을 영상으로 볼 수 있는 경로
	WorkFile    string   `db:"work_file"`    // 이 결과물을 만든 작업 파일
}

var versionDBKey string = strings.Join(dbKeys(&Version{}), ", ")
var versionDBIdx string = strings.Join(dbIdxs(&Version{}), ", ")
var _ []interface{} = dbVals(&Version{})

// ID는 Version의 고유 아이디이다. 다른 어떤 항목도 같은 아이디를 가지지 않는다.
func (v *Version) ID() string {
	return v.Show + "/" + v.Group + "/" + v.Unit + "/" + v.Task + "/" + v.Version
}

// UnitID는 부모 유닛의 아이디를 반환한다.
func (v *Version) UnitID() string {
	return v.Show + "/" + v.Group + "/" + v.Unit
}

// TaskID는 부모 태스크의 아이디를 반환한다.
func (v *Version) TaskID() string {
	return v.Show + "/" + v.Group + "/" + v.Unit + "/" + v.Task
}

// reVersionName은 가능한 버전명을 정의하는 정규식이다.
var reVersionName = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

// verifyVersionName은 받아들인 버전 이름이 유효하지 않다면 에러를 반환한다.
func verifyVersionName(version string) error {
	if !reVersionName.MatchString(version) {
		return BadRequest("invalid version name: %s", version)
	}
	return nil
}

// SplitVersionID는 받아들인 버전 아이디를 쇼, 카테고리, 유닛, 태스크, 버전으로 분리해서 반환한다.
// 만일 버전 아이디가 유효하지 않다면 에러를 반환한다.
func SplitVersionID(id string) (string, string, string, string, string, error) {
	ns := strings.Split(id, "/")
	if len(ns) != 5 {
		return "", "", "", "", "", BadRequest("invalid version id: %s", id)
	}
	show := ns[0]
	grp := ns[1]
	unit := ns[2]
	task := ns[3]
	version := ns[4]
	if show == "" || grp == "" || unit == "" || task == "" || version == "" {
		return "", "", "", "", "", BadRequest("invalid version id: %s", id)
	}
	return show, grp, unit, task, version, nil
}

func JoinVersionID(show, grp, unit, task, ver string) string {
	return show + "/" + grp + "/" + unit + "/" + task + "/" + ver
}

// verifyVersionPrimaryKeys는 받아들인 버전 아이디가 유효하지 않다면 에러를 반환한다.
func verifyVersionPrimaryKeys(show, grp, unit, task, version string) error {
	err := verifyShowName(show)
	if err != nil {
		return err
	}
	err = verifyGroupName(grp)
	if err != nil {
		return err
	}
	err = verifyUnitName(unit)
	if err != nil {
		return err
	}
	err = verifyTaskName(task)
	if err != nil {
		return err
	}
	err = verifyVersionName(version)
	if err != nil {
		return err
	}
	return nil
}

// verifyVersion은 받아들인 버전이 유효하지 않다면 에러를 반환한다.
// 필요하다면 db의 정보와 비교하거나 유효성 확보를 위해 정보를 수정한다.
func verifyVersion(db *sql.DB, v *Version) error {
	if v == nil {
		return fmt.Errorf("nil version")
	}
	err := verifyVersionPrimaryKeys(v.Show, v.Group, v.Unit, v.Task, v.Version)
	if err != nil {
		return err
	}
	return nil
}

// AddVersion은 db의 특정 프로젝트, 특정 샷에 태스크를 추가한다.
func AddVersion(db *sql.DB, v *Version) error {
	err := verifyVersion(db, v)
	if err != nil {
		return err
	}
	// 부모가 있는지 검사
	_, err = GetTask(db, v.Show, v.Group, v.Unit, v.Task)
	if err != nil {
		return err
	}
	stmts := []dbStatement{
		dbStmt(fmt.Sprintf("INSERT INTO versions (%s) VALUES (%s)", versionDBKey, versionDBIdx), dbVals(v)...),
		dbStmt("UPDATE tasks SET (working_version) = ($1) WHERE show=$2 AND unit=$3 AND task=$4", v.Version, v.Show, v.Unit, v.Task),
	}
	return dbExec(db, stmts)
}

// UpdateVersion은 db의 특정 태스크를 업데이트 한다.
func UpdateVersion(db *sql.DB, v *Version) error {
	err := verifyVersion(db, v)
	if err != nil {
		return err
	}
	_, err = GetVersion(db, v.Show, v.Group, v.Unit, v.Task, v.Version)
	if err != nil {
		return err
	}
	stmts := []dbStatement{
		dbStmt(fmt.Sprintf("UPDATE versions SET (%s) = (%s) WHERE show='%s' AND grp='%s' AND unit='%s' AND task='%s' AND version='%s'", versionDBKey, versionDBIdx, v.Show, v.Group, v.Unit, v.Task, v.Version), dbVals(v)...),
	}
	return dbExec(db, stmts)
}

// GetVersion은 db에서 하나의 버전을 찾는다.
// 해당 버전이 없다면 nil과 NotFound 에러를 반환한다.
func GetVersion(db *sql.DB, show, grp, unit, task, ver string) (*Version, error) {
	err := verifyVersionPrimaryKeys(show, grp, unit, task, ver)
	if err != nil {
		return nil, err
	}
	_, err = GetTask(db, show, grp, unit, task)
	if err != nil {
		return nil, err
	}
	stmt := dbStmt(fmt.Sprintf("SELECT %s FROM versions WHERE show=$1 AND grp=$2 AND unit=$3 AND task=$4 AND version=$5 LIMIT 1", versionDBKey), show, grp, unit, task, ver)
	v := &Version{}
	err = dbQueryRow(db, stmt, func(row *sql.Row) error {
		return scan(row, v)
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, NotFound("version not found: %s", JoinVersionID(show, grp, unit, task, ver))
		}
		return nil, err
	}
	return v, err
}

// TaskVersions는 db에서 특정 태스크의 버전 전체를 검색해 반환한다.
func TaskVersions(db *sql.DB, show, grp, unit, task string) ([]*Version, error) {
	err := verifyTaskPrimaryKeys(show, grp, unit, task)
	if err != nil {
		return nil, err
	}
	_, err = GetTask(db, show, grp, unit, task)
	if err != nil {
		return nil, err
	}
	stmt := dbStmt(fmt.Sprintf("SELECT %s FROM versions WHERE show=$1 AND grp=$2 AND unit=$3 AND task=$4", versionDBKey), show, grp, unit, task)
	versions := make([]*Version, 0)
	err = dbQuery(db, stmt, func(rows *sql.Rows) error {
		v := &Version{}
		err := scan(rows, v)
		if err != nil {
			return err
		}
		versions = append(versions, v)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(versions, func(i, j int) bool {
		return strings.Compare(versions[i].Version, versions[j].Version) < 0
	})
	return versions, nil
}

// DeleteVersion은 해당 버전과 그 하위의 모든 데이터를 db에서 지운다.
// 만일 처리 중간에 에러가 나면 아무 데이터도 지우지 않고 에러를 반환한다.
func DeleteVersion(db *sql.DB, show, grp, unit, task, ver string) error {
	_, err := GetVersion(db, show, grp, unit, task, ver)
	if err != nil {
		return err
	}
	stmts := []dbStatement{
		dbStmt("DELETE FROM versions WHERE show=$1 AND grp=$2 AND unit=$3 AND task=$4 AND version=$5", show, grp, unit, task, ver),
	}
	return dbExec(db, stmts)
}
