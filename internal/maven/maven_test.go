package maven

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// repo is a fake Maven repository.
var repo = map[string]string{
	"/org/ex/lib-a/1.0/lib-a-1.0.pom": `<project><groupId>org.ex</groupId><artifactId>lib-a</artifactId><version>1.0</version><packaging>aar</packaging>
<parent><groupId>org.ex</groupId><artifactId>parent</artifactId><version>3</version></parent>
<dependencies>
  <dependency><groupId>org.ex</groupId><artifactId>lib-b</artifactId><version>${b.version}</version></dependency>
  <dependency><groupId>org.ex</groupId><artifactId>lib-m</artifactId></dependency>
  <dependency><groupId>junit</groupId><artifactId>junit</artifactId><version>4</version><scope>test</scope></dependency>
  <dependency><groupId>org.ex</groupId><artifactId>opt</artifactId><version>1</version><optional>true</optional></dependency>
</dependencies></project>`,
	"/org/ex/parent/3/parent-3.pom": `<project><groupId>org.ex</groupId><artifactId>parent</artifactId><version>3</version><packaging>pom</packaging>
<properties><b.version>[1.1]</b.version></properties>
<dependencyManagement><dependencies><dependency><groupId>org.ex</groupId><artifactId>lib-m</artifactId><version>2.5</version></dependency></dependencies></dependencyManagement>
</project>`,
	"/org/ex/lib-b/1.1/lib-b-1.1.pom": `<project><groupId>org.ex</groupId><artifactId>lib-b</artifactId><version>1.1</version>
<dependencies><dependency><groupId>org.ex</groupId><artifactId>lib-d</artifactId><version>1.0</version></dependency></dependencies></project>`,
	"/org/ex/lib-m/2.5/lib-m-2.5.pom": `<project><groupId>org.ex</groupId><artifactId>lib-m</artifactId><version>2.5</version></project>`,
	"/org/ex/lib-e/1.0/lib-e-1.0.pom": `<project><groupId>org.ex</groupId><artifactId>lib-e</artifactId><version>1.0</version><packaging>aar</packaging>
<dependencies><dependency><groupId>org.ex</groupId><artifactId>lib-d</artifactId><version>2.0</version></dependency></dependencies></project>`,
	"/org/ex/lib-d/1.0/lib-d-1.0.pom": `<project><groupId>org.ex</groupId><artifactId>lib-d</artifactId><version>1.0</version></project>`,
	"/org/ex/lib-d/2.0/lib-d-2.0.pom": `<project><groupId>org.ex</groupId><artifactId>lib-d</artifactId><version>2.0</version></project>`,
	"/org/ex/lib-a/1.0/lib-a-1.0.aar": "aar-a",
	"/org/ex/lib-b/1.1/lib-b-1.1.jar": "jar-b",
	"/org/ex/lib-m/2.5/lib-m-2.5.jar": "jar-m",
	"/org/ex/lib-e/1.0/lib-e-1.0.aar": "aar-e",
	"/org/ex/lib-d/2.0/lib-d-2.0.jar": "jar-d2",
}

func TestResolve(t *testing.T) {
	var fetched []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := repo[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		fetched = append(fetched, r.URL.Path)
		w.Write([]byte(body))
	}))
	defer srv.Close()
	empty := httptest.NewServer(http.NotFoundHandler())
	defer empty.Close()
	r := &Resolver{Repos: []string{empty.URL, srv.URL}, Cache: t.TempDir()}
	a, _ := Parse("org.ex:lib-a:1.0")
	e, _ := Parse("org.ex:lib-e:1.0")
	arts, err := r.Resolve([]Coord{a, e})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, x := range arts {
		b, _ := os.ReadFile(x.Path)
		got = append(got, x.String()+"="+string(b))
	}
	want := "org.ex:lib-a:1.0=aar-a org.ex:lib-b:1.1=jar-b org.ex:lib-d:2.0=jar-d2 org.ex:lib-e:1.0=aar-e org.ex:lib-m:2.5=jar-m"
	if strings.Join(got, " ") != want {
		t.Errorf("resolved\n%s\nwant\n%s", strings.Join(got, " "), want)
	}
	// A second resolve uses the cache.
	n := len(fetched)
	r2 := &Resolver{Repos: []string{srv.URL}, Cache: r.Cache}
	if _, err := r2.Resolve([]Coord{a, e}); err != nil {
		t.Fatal(err)
	}
	if len(fetched) != n {
		t.Errorf("%d more downloads with a warm cache", len(fetched)-n)
	}
}

func TestCompareVersions(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"1.10", "1.9", 1}, {"2.0", "2.0.0", 0}, {"1.0-alpha", "1.0", -1}, {"24.1.0", "24.0.9", 1}} {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compare(%s, %s) = %d", c.a, c.b, got)
		}
	}
}
