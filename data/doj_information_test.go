package data_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "gogen/data"
	"gogen/matchers"
	. "gogen/test_fixtures"

	"io/ioutil"
	"path"
	"time"
)

type testEligibilityFlow struct {
}

func (ef testEligibilityFlow) BeginEligibilityFlow(info *EligibilityInfo, row *DOJRow, subject *Subject) {
	ef.EligibleDismissal(info, "Because")
}

func (ef testEligibilityFlow) EligibleDismissal(info *EligibilityInfo, reason string) {
	info.EligibilityDetermination = "Test is Eligible"
	info.EligibilityReason = reason
}

func (ef testEligibilityFlow) ProcessSubject(subject *Subject, comparisonTime time.Time, county string) map[int]*EligibilityInfo {
	infos := make(map[int]*EligibilityInfo)
	for _, conviction := range subject.Convictions {
		if ef.checkRelevancy(conviction.CodeSection, conviction.County) {
			info := NewEligibilityInfo(conviction, subject, comparisonTime, county)
			ef.BeginEligibilityFlow(info, conviction, subject)
			infos[conviction.Index] = info
		}
	}
	return infos
}

func (ef testEligibilityFlow) checkRelevancy(codeSection string, county string) bool {
	return matchers.IsProp64Charge(codeSection)
}

func (ef testEligibilityFlow) ChecksRelatedCharges() bool {
	return false
}

var _ = Describe("DojInformation", func() {
	var (
		county            string
		pathToDOJ         string
		comparisonTime    time.Time
		err               error
		testEligibilities map[int]*EligibilityInfo
		dojInformation    *DOJInformation
		dojEligibilities  map[int]*EligibilityInfo
	)

	BeforeEach(func() {
		_, err = ioutil.TempDir("/tmp", "gogen")
		Expect(err).ToNot(HaveOccurred())

		inputPath := path.Join("..", "test_fixtures", "configurable_flow.xlsx")
		pathToDOJ, _, err = ExtractFullCSVFixtures(inputPath)
		Expect(err).ToNot(HaveOccurred())

		comparisonTime = time.Date(2019, time.November, 11, 0, 0, 0, 0, time.UTC)
		testFlow := testEligibilityFlow{}
		configurableFlow, _ := NewConfigurableEligibilityFlow(EligibilityOptions{
			BaselineEligibility: BaselineEligibility{
				Dismiss: []string{"11357", "11358"},
				Reduce:  []string{"11359", "11360"},
			},
			AdditionalRelief: AdditionalRelief{
				SubjectUnder21AtConviction:    true,
				SubjectAgeThreshold:           57,
				YearsSinceConvictionThreshold: 10,
				SubjectIsDeceased:             true,
			},
		}, county)
		dojInformation, _ = NewDOJInformation(pathToDOJ, comparisonTime, configurableFlow)

		testEligibilities = dojInformation.DetermineEligibility(county, testFlow)
		dojEligibilities = dojInformation.DetermineEligibility(county, configurableFlow)

	})

	Context("when no convictions for county", func() {
		BeforeEach(func() {
			county = "LAKE"
		})

		It("Exits succesfully with dummy datetime", func() {
			Expect(dojInformation.EarliestProp64ConvictionDateInThisCounty(county)).To(BeTemporally("~", time.Now(), time.Second))
		})
	})

	Context("when convictions for county", func() {
		BeforeEach(func() {
			county = "SACRAMENTO"
		})

		It("Uses the provided eligibility flow", func() {
			Expect(testEligibilities[11].EligibilityDetermination).To(Equal("Test is Eligible"))
			Expect(testEligibilities[11].CaseNumber).To(Equal("140194; 140195"))
		})

		It("Populates ToBeRemovedEligibilities based on Index of Conviction", func() {
			Expect(dojEligibilities[11].EligibilityDetermination).To(Equal("Eligible for Dismissal"))
			Expect(dojEligibilities[11].CaseNumber).To(Equal("140194; 140195"))
		})

		Context("Computing Aggregate Statistics for convictions", func() {
			It("Counts total number of rows in file", func() {
				Expect(dojInformation.TotalRows()).To(Equal(38))
			})

			It("Counts total convictions", func() {
				Expect(dojInformation.TotalConvictions()).To(Equal(30))
			})

			It("Counts Prop64 convictions in this county sorted by code section", func() {
				Expect(dojInformation.Prop64ConvictionsInThisCountyByCodeSection(county)).To(Equal(map[string]int{"11357": 3, "11358": 7, "11359": 8}))
			})

			It("Finds the date of the earliest Prop64 conviction in the county", func() {
				expectedDate := time.Date(1979, 6, 1, 0, 0, 0, 0, time.UTC)
				Expect(dojInformation.EarliestProp64ConvictionDateInThisCounty(county)).To(Equal(expectedDate))
			})

			It("Prop64 convictions in this county by eligibility determination and reason", func() {
				Expect(dojInformation.Prop64ConvictionsInThisCountyByEligibilityByReason(county, dojEligibilities)).To(Equal(
					map[string]map[string]int{
						"Eligible for Dismissal": {
							"Dismiss all HS 11357 convictions":         2,
							"57 years or older":                        2,
							"Individual is deceased":                   1,
							"Dismiss all HS 11358 convictions":         6,
							"Conviction occurred 10 or more years ago": 1,
							"Misdemeanor or Infraction":                3,
							"21 years or younger":                      1,},
						"Eligible for Reduction": {
							"Reduce all HS 11359 convictions": 2,
						},
					}))
			})

			Context("Computing aggregate statistics for individuals", func() {
				Context("After eligibility is run", func() {
					It("Calculates individuals who will no longer have a felony ", func() {
						Expect(dojInformation.CountIndividualsNoLongerHaveFelony(dojEligibilities)).To(Equal(5))
					})

					It("Calculates individuals who no longer have any conviction", func() {
						Expect(dojInformation.CountIndividualsNoLongerHaveConviction(dojEligibilities)).To(Equal(4))
					})

					It("Calculates individuals who no longer have any conviction in the last 7 years", func() {
						Expect(dojInformation.CountIndividualsNoLongerHaveConvictionInLast7Years(dojEligibilities)).To(Equal(2))
					})

					It("Calculates individuals who will have some relief", func() {
						Expect(dojInformation.CountIndividualsWithSomeRelief(dojEligibilities)).To(Equal(12))
					})
				})

			})
		})
	})
})
